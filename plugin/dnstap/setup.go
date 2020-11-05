package dnstap

import (
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
)

var log = clog.NewWithPlugin("dnstap")

func init() { plugin.Register("dnstap", setup) }

func parseConfig(c *caddy.Controller) (Dnstap, error) {
	c.Next() // directive name
	d := Dnstap{}
	endpoint := ""

	if !c.Args(&endpoint) {
		return d, c.ArgErr()
	}

	if strings.HasPrefix(endpoint, "tcp://") {
		// remote IP endpoint
		servers, err := parse.HostPortOrFile(endpoint[6:])
		if err != nil {
			return d, c.ArgErr()
		}
		dio := newIO("tcp", servers[0])
		d = Dnstap{io: dio}
	} else {
		endpoint = strings.TrimPrefix(endpoint, "unix://")
		dio := newIO("unix", endpoint)
		d = Dnstap{io: dio}
	}

	d.IncludeRawMessage = c.NextArg() && c.Val() == "full"

	return d, nil
}

func setup(c *caddy.Controller) error {
	dnstap, err := parseConfig(c)
	if err != nil {
		return plugin.Error("dnstap", err)
	}

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

	dnsserver.GetConfig(c).AddPlugin(
		func(next plugin.Handler) plugin.Handler {
			dnstap.Next = next
			return dnstap
		})

	return nil
}
