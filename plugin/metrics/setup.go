package metrics

import (
	"net"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("prometheus", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	m, err := prometheusParse(c)
	if err != nil {
		return plugin.Error("prometheus", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		m.Next = next
		return m
	})

	c.OnStartup(m.OnStartup)
	return nil
}

func prometheusParse(c *caddy.Controller) (*Metrics, error) {
	var addr = defaultAddr
	var zones []string

	for c.Next() {
		if len(zones) > 0 {
			return nil, c.Err("can only have one metrics module per server")
		}

		for _, z := range c.ServerBlockKeys {
			zones = append(zones, plugin.Host(z).Normalize())
		}
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			_, _, e := net.SplitHostPort(addr)
			if e != nil {
				return nil, e
			}
		default:
			return nil, c.ArgErr()
		}
	}

	m := New(addr)
	for _, z := range zones {
		m.AddZone(z)
	}
	return m, nil
}

// defaultAddr is the address the where the metrics are exported by default.
const defaultAddr = "localhost:9153"
