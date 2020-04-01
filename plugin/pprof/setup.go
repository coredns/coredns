package pprof

import (
	"net"
	"strconv"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/caddyserver/caddy"
)

const pluginName = "pprof"

var log = clog.NewWithPlugin(pluginName)

const defaultAddr = "localhost:6053"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	h := &handler{addr: defaultAddr}

	i := 0
	for c.Next() {
		if i > 0 {
			return plugin.Error(pluginName, plugin.ErrOnce)
		}
		i++

		args := c.RemainingArgs()
		if len(args) == 1 {
			h.addr = args[0]
			_, _, e := net.SplitHostPort(h.addr)
			if e != nil {
				return plugin.Error(pluginName, c.Errf("%v", e))
			}
		}

		if len(args) > 1 {
			return plugin.Error(pluginName, c.ArgErr())
		}

		for c.NextBlock() {
			switch c.Val() {
			case "block":
				args := c.RemainingArgs()
				if len(args) > 1 {
					return plugin.Error(pluginName, c.ArgErr())
				}
				h.rateBloc = 1
				if len(args) > 0 {
					t, err := strconv.Atoi(args[0])
					if err != nil {
						return plugin.Error(pluginName, c.Errf("property '%s' invalid integer value '%v'", "block", args[0]))
					}
					h.rateBloc = t
				}
			default:
				return plugin.Error(pluginName, c.Errf(plugin.UnknownPropertyErrmsg, c.Val()))
			}
		}

	}

	c.OnStartup(h.Startup)
	c.OnShutdown(h.Shutdown)
	return nil
}
