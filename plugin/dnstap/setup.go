package dnstap

import (
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/dnstapio"
	"github.com/coredns/coredns/plugin/pkg/parse"
)

func init() { plugin.Register("dnstap", setup) }

type config struct {
	proto  string
	target string
	full   bool
}

func parseConfig(d *caddy.Controller) (c config, err error) {
	d.Next() // directive name

	if !d.Args(&c.target) {
		return c, d.ArgErr()
	}

	if strings.HasPrefix(c.target, "tcp://") {
		// remote IP endpoint
		servers, err := parse.HostPortOrFile(c.target[6:])
		if err != nil {
			return c, d.ArgErr()
		}
		c.target = servers[0]
		c.proto = "tcp"
	} else {
		c.target = strings.TrimPrefix(c.target, "unix://")
		c.proto = "unix"
	}

	c.full = d.NextArg() && d.Val() == "full"

	return
}

func setup(c *caddy.Controller) error {
	conf, err := parseConfig(c)
	if err != nil {
		return plugin.Error("dnstap", err)
	}

	dio := dnstapio.New(conf.proto, conf.target)
	dnstap := Dnstap{io: dio, IncludeRawMessage: conf.full}

	c.OnStartup(func() error {
		dio.Connect()
		return nil
	})

	c.OnRestart(func() error {
		dio.Close()
		return nil
	})

	c.OnFinalShutdown(func() error {
		dio.Close()
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(
		func(next plugin.Handler) plugin.Handler {
			dnstap.Next = next
			return dnstap
		})

	return nil
}
