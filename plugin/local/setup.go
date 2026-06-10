package local

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("local", setup) }

func setup(c *caddy.Controller) error {
	c.Next() // 'local'
	if c.NextArg() {
		return plugin.Error("local", c.ArgErr())
	}
	if c.NextBlock() {
		return plugin.Error("local", c.Errf("unknown property '%s'", c.Val()))
	}

	l := Local{}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		l.Next = next
		return l
	})

	return nil
}
