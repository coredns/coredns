package azure

import (
	"context"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("azure")

func init() { plugin.Register("azure", setup) }

func setup(c *caddy.Controller) error {
	zoneList, fall, err := parse(c)
	if err != nil {
		return plugin.Error("azure", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	h, err := New(ctx, zoneList)
	if err != nil {
		cancel()
		return plugin.Error("azure", err)
	}
	h.Fall = fall
	if err := h.Run(ctx); err != nil {
		cancel()
		return plugin.Error("azure", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})
	c.OnShutdown(func() error { cancel(); return nil })
	return nil
}

func parse(c *caddy.Controller) (zones, fall.F, error) {
	var fall fall.F

	zoneList := zones{}
	uniqueZones := map[string]struct{}{}

	for c.Next() {
		var zoneData zone
		var resourceGroup string
		var zoneName string

		args := c.RemainingArgs()

		if len(args) > 1 {
			return zoneList, fall, c.Errf("invalid resource group/zone: %q", args)
		}

		for i := 0; i < len(args); i++ {
			parts := strings.Split(args[i], ":")
			if len(parts) != 2 {
				return zoneList, fall, c.Errf("invalid resource group/zone: %q", args[i])
			}
			resourceGroup, zoneName = parts[0], parts[1]
			if resourceGroup == "" || zoneName == "" {
				return zoneList, fall, c.Errf("invalid resource group/zone: %q", args[i])
			}
		}

		zoneData.resourceGroup = resourceGroup
		zoneData.zone = zoneName

		for c.NextBlock() {
			switch c.Val() {
			case "subscription":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				zoneData.subscriptionID = c.Val()
			case "tenant":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				zoneData.tenantID = c.Val()
			case "client":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				zoneData.clientID = c.Val()
			case "secret":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				zoneData.clientSecret = c.Val()
			case "environment":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				if !isValidCloudType(c.Val()) {
					return zoneList, fall, c.Errf("cannot set azure environment, invalid environment: %s", c.Val())
				}
				// convert to uppercase so we don't have to deal with all the different variations
				// between the old go sdk and newer one
				zoneData.environment = strings.ToUpper(c.Val())
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			case "access":
				if !c.NextArg() {
					return zoneList, fall, c.ArgErr()
				}
				access := c.Val()
				if access == "private" {
					zoneData.private = true
				} else if access == "public" || access == "" {
					// if access is set to public or in the case it is not specified assume public
					zoneData.private = false
				} else {
					return zoneList, fall, c.Errf("invalid access value: can be public/private, found: %s", access)
				}
			default:
				return zoneList, fall, c.Errf("unknown property: %q", c.Val())
			}
		}
		// Check for duplicate / conflicting zones check that used to be here elsewhere
		zKey := strings.Join([]string{zoneData.subscriptionID, resourceGroup, zoneName}, "#")
		if _, ok := uniqueZones[zKey]; ok {
			return zoneList, fall, c.Errf("conflicting zone: %q", zKey)
		}
		uniqueZones[zKey] = struct{}{}

		fqdn := dns.Fqdn(zoneName)
		zoneList[fqdn] = append(zoneList[fqdn], &zoneData)
	}

	return zoneList, fall, nil
}
