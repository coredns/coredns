package k8s_node

import (
	"strconv"

	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("k8s_node")

func init() { plugin.Register("k8s_node", setup) }

func setup(c *caddy.Controller) error {
	e, err := parse(c)
	if err != nil {
		return plugin.Error("k8s_node", err)
	}

	// Do this in OnStartup, so all plugins have been initialized.
	c.OnStartup(func() error {
		m := dnsserver.GetConfig(c).Handler("kubernetes")
		if m == nil {
			return nil
		}
		if x, ok := m.(Nodeer); ok {
			e.nodeFunc = x.Node
			e.nodeAddrFunc = x.NodeAddress
		}
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		e.Next = next
		return e
	})

	return nil
}

func parse(c *caddy.Controller) (*Node, error) {
	e := New()

	for c.Next() { // external
		zones := c.RemainingArgs()
		e.Zones = zones
		for i, str := range e.Zones {
			e.Zones[i] = plugin.Host(str).Normalize()
		}
		for c.NextBlock() {
			switch c.Val() {
			case "ttl":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				t, err := strconv.Atoi(args[0])
				if err != nil {
					return nil, err
				}
				if t < 0 || t > 3600 {
					return nil, c.Errf("ttl must be in range [0, 3600]: %d", t)
				}
				e.ttl = uint32(t)
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return e, nil
}
