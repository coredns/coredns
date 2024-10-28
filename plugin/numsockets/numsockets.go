package numsockets

import (
	"fmt"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

// defaultNumSockets is the default number of sockets that will listen on one port.
const defaultNumSockets = 2

func init() { plugin.Register("numsockets", setup) }

func setup(c *caddy.Controller) error {
	err := parseNumSockets(c)
	if err != nil {
		return plugin.Error("numsockets", err)
	}
	return nil
}

func parseNumSockets(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)
	c.Next() // "numsockets"

	args := c.RemainingArgs()

	if len(args) > 1 || c.Next() {
		return c.ArgErr()
	}

	if len(args) == 0 {
		// Nothing specified; use default
		config.NumSockets = defaultNumSockets
		return nil
	}

	numSockets, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid num sockets: %w", err)
	}
	if numSockets < 1 {
		return fmt.Errorf("num sockets can not be zero or negative: %d", numSockets)
	}
	config.NumSockets = numSockets

	return nil
}
