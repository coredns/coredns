package shed

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	pkgparse "github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

const pluginName = "shed"

var log = clog.NewWithPlugin(pluginName)

// stackDepth is a burst budget, not a knob: ~12-16ms at a typical socket's
// serialized drain rate; a larger value would only hold staler responses.
const stackDepth = 1024

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	s, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	// The hook only exists on the plain-DNS UDP path; on any other transport
	// shed would silently protect nothing, so refuse at parse time. Every
	// block key is checked: caddy propagates the plugin list to all keys of
	// a server block, not just the one setup runs for.
	for _, key := range c.ServerBlockKeys {
		if tr, _ := pkgparse.Transport(key); tr != transport.DNS {
			return plugin.Error(pluginName, c.Errf("only plain DNS server blocks are supported; %q uses transport %q", key, tr))
		}
	}

	cfg := dnsserver.GetConfig(c)
	cfg.UDPDecorateWriterFunc = s.decorateWriterFactory

	// On a graceful reload the retiring instance must stop its writer
	// goroutines; shutdown removes only this instance's sockets.
	c.OnShutdown(s.shutdown)

	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		s.Next = next
		return s
	})
	return nil
}

func parse(c *caddy.Controller) (*Shed, error) {
	s := &Shed{}
	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		if len(c.RemainingArgs()) != 0 {
			return nil, c.ArgErr()
		}
		if c.NextBlock() {
			return nil, c.Errf("shed takes no options")
		}
	}
	return s, nil
}
