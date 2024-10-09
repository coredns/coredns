package numsockets

import (
	"fmt"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

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

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 1 {
			return c.ArgErr()
		}
		numSockets, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid num sockets: %w", err)
		}
		if numSockets < 1 {
			return fmt.Errorf("num sockets can not be zero or negative: %d", numSockets)
		}
		config.NumSockets = numSockets
	}
	return nil
}
