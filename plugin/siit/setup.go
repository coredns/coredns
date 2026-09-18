package siit

import (
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"
)

const pluginName = "siit"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	siit, err := siitParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		siit.Next = next
		return siit
	})

	return nil
}

func siitParse(c *caddy.Controller) (*SIIT, error) {
	_, defaultPref6, _ := net.ParseCIDR("64:ff9b::/96")
	siit := &SIIT{
		Upstream:   upstream.New(),
		IPv6Prefix: defaultPref6,
	}

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) > 0 {
			return nil, c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "ipv6_prefix":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				pref, err := parseIpv6Prefix(c, c.Val())

				if err != nil {
					return nil, err
				}
				siit.IPv6Prefix = pref
			case "eam":
				pref, err := parseEam(c)

				if err != nil {
					return nil, err
				}

				if siit.Eam4 == nil {
					siit.Eam4 = make(map[string]net.IP)
				}

				siit.Eam4[pref[1].String()] = pref[0]
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return siit, nil
}

func parseIpv6Prefix(c *caddy.Controller, addr string) (*net.IPNet, error) {
	ip, pref, err := net.ParseCIDR(addr)
	if err != nil {
		return nil, err
	}

	// Test for valid prefix
	n, total := pref.Mask.Size()
	if total != 128 {
		return nil, c.Errf("invalid netmask %d IPv6 address: %q", total, pref)
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return nil, c.Errf("invalid IPv6 prefix %q", addr)
	}

	// RFC 6052 §2.2: only these lengths are valid.
	switch n {
	case 32, 40, 48, 56, 64, 96:
		// ok
	default:
		return nil, c.Errf("invalid prefix length %q: must be one of /32, /40, /48, /56, /64, /96", pref)
	}

	// RFC 6052 §2.2: byte 8 (bits 64-71, the "u" octet) is reserved and
	// MUST be zero for every valid prefix length — including /96, where
	// it's still part of the operator-configured prefix bits.
	if pref.IP.To16()[8] != 0 {
		return nil, c.Errf("invalid prefix %q: reserved octet (byte 8) must be zero", pref)
	}

	return pref, nil
}

func parseEam(c *caddy.Controller) (map[int]net.IP, error) {
	args := c.RemainingArgs()
	if len(args) != 2 {
		return nil, c.ArgErr()
	}

	pref0 := net.ParseIP(args[0])
	if pref0 == nil {
		return nil, c.Errf("invalid IP address: %q", pref0)
	}

	if pref0.To4() == nil {
		return nil, c.Errf("invalid IPv4 address: %q", pref0)
	}

	pref1 := net.ParseIP(args[1])
	if pref1 == nil {
		return nil, c.Errf("invalid IP address: %q", pref1)
	}

	if pref1.To4() != nil {
		return nil, c.Errf("invalid IPv6 address: %q", pref1)
	}

	pref := make(map[int]net.IP)
	pref[0] = pref0
	pref[1] = pref1
	return pref, nil
}
