package eureka

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/caddyserver/caddy"
)

var log = clog.NewWithPlugin("eureka")

var f = func(baseUrl string) clientAPI {
	return client{
		BaseUrl: baseUrl,
	}
}

func init() {
	plugin.Register("eureka", setup)
}

func setup(c *caddy.Controller) error {
	for c.Next() {
		zones := c.RemainingArgs()

		var fall fall.F
		var eurekaBaseUrl string

		eurekaOptions := &options{
			refresh: time.Duration(1) * time.Minute, // default update frequency to 1 minute
			ttl:     30,                             // default TTL to 30 seconds
			mode:    "",
		}

		for c.NextBlock() {
			switch c.Val() {
			case "base_url":
				v := c.RemainingArgs()
				if len(v) != 1 {
					return plugin.Error("eureka", c.Errf("invalid base_url '%v'", v))
				}
				eurekaBaseUrl = v[0]
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			case "refresh":
				if c.NextArg() {
					refreshStr := c.Val()
					_, err := strconv.Atoi(refreshStr)
					if err == nil {
						refreshStr = fmt.Sprintf("%ss", c.Val())
					}
					eurekaOptions.refresh, err = time.ParseDuration(refreshStr)
					if err != nil {
						return plugin.Error("eureka", c.Errf("Unable to parse duration: '%v'", err))
					}
					if eurekaOptions.refresh <= 0 {
						return plugin.Error("eureka", c.Errf("refresh interval must be greater than 0: %s", refreshStr))
					}
				} else {
					return plugin.Error("eureka", c.ArgErr())
				}
			case "ttl":
				remaining := c.RemainingArgs()
				if len(remaining) < 1 {
					return plugin.Error("eureka", c.Err("ttl needs a time in second"))
				}
				ttl, err := strconv.Atoi(remaining[0])
				if err != nil {
					return plugin.Error("eureka", c.Err("ttl needs a number of second"))
				}
				if ttl <= 0 || ttl > 65535 {
					return plugin.Error("eureka", c.Err("ttl provided is invalid"))
				}
				eurekaOptions.ttl = uint32(ttl)
			case "mode":
				if c.NextArg() {
					v := c.Val()
					eurekaOptions.mode = mode(v)
					if eurekaOptions.mode != modeVip && eurekaOptions.mode != modeApp {
						return plugin.Error("eureka", c.Errf("invalid mode '%v'", v))
					}
				} else {
					return plugin.Error("eureka", c.ArgErr())
				}
			default:
				return plugin.Error("eureka", c.Errf("unknown property '%s'", c.Val()))
			}
		}
		if eurekaOptions.mode == "" {
			return plugin.Error("eureka", c.Err("mode is required"))
		}
		if eurekaBaseUrl == "" {
			return plugin.Error("eureka", c.Err("base_url is required"))
		}
		ctx := context.Background()
		client := f(eurekaBaseUrl)
		e, err := New(ctx, eurekaOptions, client)

		if err != nil {
			return plugin.Error("eureka", c.Errf("failed to create Eureka plugin: %v", err))
		}

		e.Zones = zones
		if len(zones) == 0 {
			e.Zones = make([]string, len(c.ServerBlockKeys))
			copy(e.Zones, c.ServerBlockKeys)
		}
		for i, str := range e.Zones {
			e.Zones[i] = plugin.Host(str).Normalize()
		}
		e.Fall = fall
		if err := e.Run(ctx); err != nil {
			return plugin.Error("eureka", c.Errf("failed to initialize Eureka plugin: %v", err))
		}
		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			e.Next = next
			return e
		})
	}
	return nil
}
