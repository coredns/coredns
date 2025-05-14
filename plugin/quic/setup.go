package quic

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() {
	caddy.RegisterPlugin("quic", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	err := parseQuic(c)
	if err != nil {
		return plugin.Error("quic", err)
	}
	return nil
}

func parseQuic(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// Skip the "quic" directive itself
	c.Next()

	// Get any arguments on the "quic" line
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
			if val <= 0 {
				return c.Errf("max_streams must be a positive integer: %d", val)
			}
			if config.MaxQUICStreams != nil {
				return c.Err("max_streams already defined for this server block")
			}
			config.MaxQUICStreams = &val
		case "worker_pool_size":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return c.ArgErr()
			}
			val, err := strconv.Atoi(args[0])
			if err != nil {
				return c.Errf("invalid worker_pool_size value '%s': %v", args[0], err)
			}
			if val <= 0 {
				return c.Errf("worker_pool_size must be a positive integer: %d", val)
			}
			if config.MaxQUICWorkerPoolSize != nil {
				return c.Err("worker_pool_size already defined for this server block")
			}
			config.MaxQUICWorkerPoolSize = &val
		default:
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}
