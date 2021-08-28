package refusenord

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

const pluginName = "refusenord"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return handler{next}
	})

	return nil
}
