package dynapi

import (
	"net"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("dynapi", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	addr, _, err := apiParse(c)
	if err != nil {
		return plugin.Error("dynapi", err)
	}

	api := newApi(addr)
	c.OnStartup(func() error {
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, p := range plugins {
			if implementable, ok := p.(DynapiImplementable); ok {
				for _, zoneName := range implementable.GetZones() {
					api.dynapiImplementable[zoneName] = implementable
				}
			}
		}
		return nil
	})
	c.OnStartup(api.OnStartup)
	c.OnRestart(api.OnRestart)
	c.OnFinalShutdown(api.OnFinalShutdown)
	return nil
}

func apiParse(c *caddy.Controller) (string, time.Duration, error) {
	addr := ""
	dur := time.Duration(0)
	for c.Next() {
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			if _, _, e := net.SplitHostPort(addr); e != nil {
				return "", 0, e
			}
		default:
			return "", 0, c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			default:
				return "", 0, c.ArgErr()
			}
		}
	}
	return addr, dur, nil
}
