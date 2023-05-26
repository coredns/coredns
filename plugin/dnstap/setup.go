package dnstap

import (
	"net/url"
	"os"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("dnstap")

func init() { plugin.Register("dnstap", setup) }

func parseConfig(c *caddy.Controller) ([]*Dnstap, error) {
	dnstaps := []*Dnstap{}

	for c.Next() { // directive name
		d := Dnstap{}
		endpoint := ""

		args := c.RemainingArgs()

		if len(args) == 0 {
			return nil, c.ArgErr()
		}

		endpoint = args[0]

		var dio *dio
		if strings.HasPrefix(endpoint, "tls://") {
			// remote network endpoint
			endpointURL, err := url.Parse(endpoint)
			if err != nil {
				return nil, c.ArgErr()
			}
			dio = newIO("tls", endpointURL.Host)
			d = Dnstap{io: dio}
		} else if strings.HasPrefix(endpoint, "tcp://") {
			// remote network endpoint
			endpointURL, err := url.Parse(endpoint)
			if err != nil {
				return nil, c.ArgErr()
			}
			dio = newIO("tcp", endpointURL.Host)
			d = Dnstap{io: dio}
		} else {
			endpoint = strings.TrimPrefix(endpoint, "unix://")
			dio = newIO("unix", endpoint)
			d = Dnstap{io: dio}
		}

		d.IncludeRawMessage = len(args) == 2 && args[1] == "full"

		hostname, _ := os.Hostname()
		d.Identity = []byte(hostname)
		d.Version = []byte(caddy.AppName + "-" + caddy.AppVersion)

		for c.NextBlock() {
			switch c.Val() {
			case "skipverify":
				{
					dio.skipVerify = true
				}
			case "identity":
				{
					if !c.NextArg() {
						return nil, c.ArgErr()
					}
					d.Identity = []byte(c.Val())
				}
			case "version":
				{
					if !c.NextArg() {
						return nil, c.ArgErr()
					}
					d.Version = []byte(c.Val())
				}
			}
		}
		dnstaps = append(dnstaps, &d)
	}
	return dnstaps, nil
}

func setup(c *caddy.Controller) error {
	dnstaps, err := parseConfig(c)
	if err != nil {
		return plugin.Error("dnstap", err)
	}

	for i := range dnstaps {
		dnstap := dnstaps[i]
		c.OnStartup(func() error {
			if err := dnstap.io.(*dio).connect(); err != nil {
				log.Errorf("No connection to dnstap endpoint: %s", err)
			}
			return nil
		})

		c.OnRestart(func() error {
			dnstap.io.(*dio).close()
			return nil
		})

		c.OnFinalShutdown(func() error {
			dnstap.io.(*dio).close()
			return nil
		})

		if i == len(dnstaps)-1 {
			// last dnstap plugin in block: point next to next plugin
			dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
				dnstap.Next = next
				return dnstap
			})
		} else {
			// not last dnstap plugin in block: point next to next dnstap
			nextDnstap := dnstaps[i+1]
			dnsserver.GetConfig(c).AddPlugin(func(plugin.Handler) plugin.Handler {
				dnstap.Next = nextDnstap
				return dnstap
			})
		}
	}

	return nil
}
