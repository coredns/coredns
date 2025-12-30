package grpc_server

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() {
	caddy.RegisterPlugin("grpc_server", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	err := parseGRPCServer(c)
	if err != nil {
		return plugin.Error("grpc_server", err)
	}
	return nil
}

func parseGRPCServer(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// Skip the "grpc_server" directive itself
	c.Next()

	// Get any arguments on the "grpc_server" line
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
			if config.MaxGRPCStreams != nil {
				return c.Err("max_streams already defined for this server block")
			}
			config.MaxGRPCStreams = &val
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
			if config.MaxGRPCConnections != nil {
				return c.Err("max_connections already defined for this server block")
			}
			config.MaxGRPCConnections = &val
		default:
			return c.Errf("unknown property '%s'", c.Val())
		}
	}

	return nil
}
