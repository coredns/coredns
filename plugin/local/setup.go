package local

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("local", setup) }

func setup(c *caddy.Controller) error {
	l, err := parse(c)
	if err != nil {
		return plugin.Error("local", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		l.Next = next
		return l
	})

	return nil
}

func parse(c *caddy.Controller) (Local, error) {
	l := Local{}

	for c.Next() {
		if len(c.RemainingArgs()) != 0 {
			return l, c.ArgErr()
		}
		for c.NextBlock() {
			switch c.Val() {
			case "localhost_prefix":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return l, c.ArgErr()
				}
				switch args[0] {
				case "on":
					l.disableLocalhostPrefix = false
				case "off":
					l.disableLocalhostPrefix = true
				default:
					return l, c.Errf("localhost_prefix expects 'on' or 'off', got %q", args[0])
				}
			default:
				return l, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return l, nil
}
