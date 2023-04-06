package external

import (
	"errors"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"
)

const pluginName = "k8s_external"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	e, err := parse(c)
	if err != nil {
		return plugin.Error("k8s_external", err)
	}

	// Do this in OnStartup, so all plugins have been initialized.
	c.OnStartup(func() error {
		m := dnsserver.GetConfig(c).Handler("kubernetes")
		if m == nil {
			return plugin.Error(pluginName, errors.New("kubernetes plugin not loaded"))
		}

		x, ok := m.(Externaler)
		if !ok {
			return plugin.Error(pluginName, errors.New("kubernetes plugin does not implement the Externaler interface"))
		}

		e.externalFunc = x.External
		e.externalAddrFunc = x.ExternalAddress
		e.externalServicesFunc = x.ExternalServices
		e.externalSerialFunc = x.ExternalSerial
		return nil
	})

	e.upstream = upstream.New()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		e.Next = next
		return e
	})

	return nil
}

func parse(c *caddy.Controller) (*External, error) {
	e := New()

	for c.Next() { // external
		e.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
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
			case "apex":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				e.apex = args[0]
			case "headless":
				e.headless = true
			case "fallthrough":
				e.Fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return e, nil
}
