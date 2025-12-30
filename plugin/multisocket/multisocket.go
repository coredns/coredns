package multisocket

import (
	"fmt"
	"runtime"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

const pluginName = "multisocket"
const maxNumSockets = 1024

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	err := parseNumSockets(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	return nil
}

func parseNumSockets(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)
	c.Next() // "multisocket"

	args := c.RemainingArgs()

	if len(args) > 1 || c.Next() {
		return c.ArgErr()
	}

	if len(args) == 0 {
		// Nothing specified; use default that is equal to GOMAXPROCS.
		config.NumSockets = runtime.GOMAXPROCS(0)
		return nil
	}

	numSockets, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid num sockets: %w", err)
	}
	if numSockets < 1 {
		return fmt.Errorf("num sockets can not be zero or negative: %d", numSockets)
	}
	if numSockets > maxNumSockets {
		return fmt.Errorf("num sockets exceeds maximum (%d): %d", maxNumSockets, numSockets)
	}
	config.NumSockets = numSockets

	return nil
}
