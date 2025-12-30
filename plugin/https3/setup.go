package https3

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() {
	caddy.RegisterPlugin("https3", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	err := parseDOH3(c)
	if err != nil {
		return plugin.Error("https3", err)
	}
	return nil
}

func parseDOH3(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// Skip the "https3" directive itself
	c.Next()

	// Get any arguments on the "https3" line
	args := c.RemainingArgs()
	if len(args) > 0 {
		return c.ArgErr()
	}

	// Process all nested directives in the block
	for c.NextBlock() {
		switch c.Val() {
		case "max_streams":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return c.ArgErr()
			}
			val, err := strconv.Atoi(args[0])
			if err != nil {
				return c.Errf("invalid max_streams value '%s': %v", args[0], err)
			}
			if val < 0 {
				return c.Errf("max_streams must be a non-negative integer: %d", val)
			}
			if config.MaxHTTPS3Streams != nil {
				return c.Err("max_streams already defined for this server block")
			}
			config.MaxHTTPS3Streams = &val
		default:
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}
