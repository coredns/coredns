package geoip

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

const pluginName = "geoip"

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	geoip, err := geoipParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		geoip.Next = next
		return geoip
	})

	return nil
}

func geoipParse(c *caddy.Controller) (*GeoIP, error) {
	var dbPath string

	for c.Next() {
		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		if dbPath != "" {
			return nil, c.Errf("configuring multiple databases is not supported")
		}
		dbPath = c.Val()
		// There shouldn't be any more arguments.
		if len(c.RemainingArgs()) != 0 {
			return nil, c.ArgErr()
		}
		// The plugin should not have any config block.
		if c.NextBlock() {
			return nil, c.Err("unexpected config block")
		}
	}

	geoIP, err := newGeoIP(dbPath)
	if err != nil {
		return geoIP, c.Err(err.Error())
	}
	return geoIP, nil
}
