package metadata

import (
	"fmt"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("metadata", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	m, err := metadataParse(c)
	if err != nil {
		return err
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		m.Next = next
		return m
	})

	c.OnStartup(func() error {
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, p := range plugins {
			if met, ok := p.(Provider); ok {
				m.Providers = append(m.Providers, met)
			}
		}
		return nil
	})

	return nil
}

func metadataParse(c *caddy.Controller) (*Metadata, error) {
	m := newMetadata()
	for c.Next() {
		zones := c.RemainingArgs()

		if len(zones) != 0 {
			m.Zones = zones
			for i := 0; i < len(m.Zones); i++ {
				m.Zones[i] = plugin.Host(m.Zones[i]).Normalize()
			}
		} else {
			m.Zones = make([]string, len(c.ServerBlockKeys))
			for i := 0; i < len(c.ServerBlockKeys); i++ {
				m.Zones[i] = plugin.Host(c.ServerBlockKeys[i]).Normalize()
			}
		}

		for c.NextBlock() {
			err := m.parseEDNS0(c)
			if err != nil {
				return nil, err
			}
		}

		if c.Next() {
			return nil, plugin.Error("metadata", c.ArgErr())
		}
	}

	return m, nil
}

func (m *Metadata) parseEDNS0(c *caddy.Controller) error {
	name := c.Val()
	args := c.RemainingArgs()
	// <label> <definition>
	// <label> edns0 <id>
	// <label> ends0 <id> <encoded-format> <params of format ...>
	// Valid encoded-format are hex (default), bytes, ip.

	argsLen := len(args)
	if argsLen != 2 && argsLen != 3 && argsLen != 6 {
		return fmt.Errorf("invalid edns0 directive")
	}
	code := args[1]

	dataType := "hex"
	size := "0"
	start := "0"
	end := "0"

	if argsLen > 2 {
		dataType = args[2]
	}

	if argsLen == 6 && dataType == "hex" {
		size = args[3]
		start = args[4]
		end = args[5]
	}

	err := m.addEDNS0Map(code, name, dataType, size, start, end)
	if err != nil {
		return fmt.Errorf("could not add EDNS0 map for %s: %s", name, err)
	}

	return nil
}
