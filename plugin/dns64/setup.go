package dns64

import (
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"
)

const pluginName = "dns64"

var log = clog.NewWithPlugin(pluginName)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	dns64, err := dns64Parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dns64.Next = next
		return dns64
	})

	return nil
}

func dns64Parse(c *caddy.Controller) (*DNS64, error) {
	_, defaultPref, _ := net.ParseCIDR("64:ff9b::/96")
	dns64 := &DNS64{
		Upstream: upstream.New(),
		Prefix:   defaultPref,
	}

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 1 {
			pref, err := parsePrefix(c, args[0])

			if err != nil {
				return nil, err
			}
			dns64.Prefix = pref
			continue
		}
		if len(args) > 0 {
			return nil, c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "prefix":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				pref, err := parsePrefix(c, c.Val())

				if err != nil {
					return nil, err
				}
				dns64.Prefix = pref
			case "translate_all":
				dns64.TranslateAll = true
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return dns64, nil
}

func parsePrefix(c *caddy.Controller, addr string) (*net.IPNet, error) {
	_, pref, err := net.ParseCIDR(addr)
	if err != nil {
		return nil, err
	}

	// Test for valid prefix
	n, total := pref.Mask.Size()
	if total != 128 {
		return nil, c.Errf("invalid netmask %d IPv6 address: %q", total, pref)
	}
	if n%8 != 0 || n < 32 || n > 96 {
		return nil, c.Errf("invalid prefix length %q", pref)
	}

	return pref, nil
}
