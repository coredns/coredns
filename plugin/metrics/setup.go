package metrics

import (
	"fmt"
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
	addr, zones, err := prometheusParse(c)
	if err != nil {
		return plugin.Error("prometheus", err)
	}

	// we start/stop listener, only if there is something to listen
	// the same address can be defined on multiple prometheus directives. Only the first one will manage the listener
	// distributor keep track of listener already booked.
	var mlsn *metricsListener
	distributor := dnsserver.GetListenerDistributor(c)
	booked, booker := distributor.IsBooked("tcp", addr)
	if booked {
		x, ok := booker.(*metricsListener)
		if !ok {
			return plugin.Error("prometheus", fmt.Errorf("the address (%s) for listening is already booked by %s (and is not recognized as prometheus)", addr, booker.Name()))
		}
		mlsn = x
	} else {
		mlsn = newListener(nil)
		alloc, err := dnsserver.GetListenerDistributor(c).BookListener("tcp", addr, mlsn, true)
		if err != nil {
			return plugin.Error("prometheus", err)
		}
		mlsn.alloc = alloc
		c.OnStartup(mlsn.OnStartup)
		c.OnShutdown(mlsn.OnShutdown)
	}

	// Whatever the listening server, we build one Metrics plugin to collect the metrics about this path of DNS Service
	m := New(mlsn, zones)
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		m.Next = next
		return m
	})

	return nil
}

func prometheusParse(c *caddy.Controller) (string, []string, error) {
	i := 0
	zones := []string{}
	addr := defaultAddr
	for c.Next() {
		if i > 0 {
			return "", nil, plugin.ErrOnce
		}
		i++

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
				return "", nil, e
			}
		default:
			return "", nil, c.ArgErr()
		}
	}
	return addr, zones, nil
}

// defaultAddr is the address the where the metrics are exported by default.
const defaultAddr = "localhost:9153"
