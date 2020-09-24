package debug

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("debug", setup) }

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	for c.Next() {
		if c.NextArg() {
			return plugin.Error("debug", c.ArgErr())
		}
		config.Debug = true
	}

	return nil
}
