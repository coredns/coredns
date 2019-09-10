package transfer

import (
	"net"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

func init() {
	caddy.RegisterPlugin("transfer", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	t, err := parse(c)

	if err != nil {
		return plugin.Error("transfer", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	c.OnStartup(func() error {
		// find all plugins that implement Transferer and add them to Transferers
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, pl := range plugins {
			tr, ok := pl.(Transferer)
			if !ok {
				continue
			}
			for _, x := range t.xfrs {
				x.Transferers = append(x.Transferers, tr)
			}
		}
		return nil
	})

	return nil
}

func parse(c *caddy.Controller) (*Transfer, error) {

	t := &Transfer{}
	for c.Next() {
		x := &xfr{}
		zones := c.RemainingArgs()

		if len(zones) != 0 {
			x.Zones = zones
			for i := 0; i < len(x.Zones); i++ {
				x.Zones[i] = plugin.Host(x.Zones[i]).Normalize()
			}
		} else {
			x.Zones = make([]string, len(c.ServerBlockKeys))
			for i := 0; i < len(c.ServerBlockKeys); i++ {
				x.Zones[i] = plugin.Host(c.ServerBlockKeys[i]).Normalize()
			}
		}

	BLOCK:
		for c.NextBlock() {
			switch c.Val() {
			case "to":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				for _, host := range args {
					if host == "*" {
						x.to = []string{"*"}
						continue BLOCK
					}
					ip := net.ParseIP(host)
					if ip == nil {
						return nil, plugin.Error("transfer", c.Errf("non-ip '%s' found in 'to'", c.Val()))
					}
					x.to = append(x.to, host)
				}
			default:
				return nil, plugin.Error("transfer", c.Errf("unknown property '%s'", c.Val()))
			}
		}
		if len(x.to) == 0 {
			return nil, plugin.Error("transfer", c.Errf("'to' is required", c.Val()))
		}
	}
	return t, nil
}
