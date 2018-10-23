package mdns

import (
	"net"
	"sync"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/mholt/caddy"
)

var log = clog.NewWithPlugin("mdns")

const defaultAddr = "0.0.0.0:5353"

func init() {
	caddy.RegisterPlugin("mdns", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	h := &handler{addr: defaultAddr}

	i := 0
	for c.Next() {
		if i > 0 {
			return plugin.Error("mdns", plugin.ErrOnce)
		}
		i++

		args := c.RemainingArgs()
		if len(args) == 1 {
			h.addr = args[0]
			_, _, e := net.SplitHostPort(h.addr)
			if e != nil {
				return e
			}
		}
		if len(args) > 1 {
			return plugin.Error("mdns", c.ArgErr())
		}
		if c.NextBlock() {
			return plugin.Error("mdns", c.ArgErr())
		}
	}

	mdnsOnce.Do(func() {
		c.OnStartup(h.Startup)
		c.OnShutdown(h.Shutdown)
	})

	return nil
}

var mdnsOnce sync.Once
