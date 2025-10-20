package ecs

import (
	"net"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("ecs", setup) }

func setup(c *caddy.Controller) error {
	c.Next()

	v4Mask := uint8(24)
	v6Mask := uint8(56)

	if c.NextBlock() {
		maskType := c.Val()
		if maskType != "mask_v4" && maskType != "mask_v6" {
			return plugin.Error("ecs", c.ArgErr())
		}
		if !c.NextArg() {
			return plugin.Error("ecs", c.ArgErr())
		}
		val := c.Val()
		valI, err := strconv.Atoi(val)
		if err != nil {
			return plugin.Error("ecs", err)
		}

		if maskType == "mask_v4" {
			if valI > 32 || valI < 0 {
				return plugin.Error("ecs", c.Err("Invalid ipv4 netmask size!"))
			}
			v4Mask = uint8(valI)
		} else {
			if valI > 128 || valI < 0 {
				return plugin.Error("ecs", c.Err("Invalid ipv6 netmask size!"))
			}
			v6Mask = uint8(valI)
		}
	}

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &Ecs{
			Next:       next,
			v4Mask:     net.CIDRMask(int(v4Mask), 32),
			v6Mask:     net.CIDRMask(int(v6Mask), 128),
			v4MaskSize: v4Mask,
			v6MaskSize: v6Mask,
		}
	})

	// All OK, return a nil error.
	return nil
}
