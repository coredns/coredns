package https

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() {
	caddy.RegisterPlugin("https", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	err := parseDOH(c)
	if err != nil {
		return plugin.Error("https", err)
	}
	return nil
}

func parseDOH(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// Skip the "https" directive itself
	c.Next()

	// Get any arguments on the "https" line
	args := c.RemainingArgs()
	if len(args) > 0 {
		return c.ArgErr()
	}

	// Process all nested directives in the block
	for c.NextBlock() {
		switch c.Val() {
		case "max_connections":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return c.ArgErr()
			}
			val, err := strconv.Atoi(args[0])
			if err != nil {
				return c.Errf("invalid max_connections value '%s': %v", args[0], err)
			}
			if val < 0 {
				return c.Errf("max_connections must be a non-negative integer: %d", val)
			}
			if config.MaxHTTPSConnections != nil {
				return c.Err("max_connections already defined for this server block")
			}
			config.MaxHTTPSConnections = &val
		default:
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}
