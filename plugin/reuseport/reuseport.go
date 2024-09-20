package reuseport

import (
	"fmt"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("reuseport", setup) }

func setup(c *caddy.Controller) error {
	err := parseNumSocks(c)
	if err != nil {
		return plugin.Error("reuseport", err)
	}
	return nil
}

func parseNumSocks(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 1 {
			return c.ArgErr()
		}
		numSocks, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid num socks: %w", err)
		}
		if numSocks < 1 {
			return fmt.Errorf("num socks can not be zero or negative: %d", numSocks)
		}
		config.NumSocks = numSocks
	}
	return nil
}
