package dynapirest

import (
	"net"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/dynapi"
	"github.com/coredns/coredns/plugin"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("dynapirest", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	addr, _, err := dynapiParse(c)
	if err != nil {
		return plugin.Error("dynapirest", err)
	}

	dynapiInstance := newDynapiRest(addr)
	c.OnStartup(func() error {
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, p := range plugins {
			if writer, ok := p.(dynapi.Writable); ok {
				for _, zoneName := range writer.GetZones() {
					dynapiInstance.dynapiWriters[zoneName] = writer
				}
			}
		}
		return nil
	})
	c.OnStartup(dynapiInstance.OnStartup)
	c.OnRestart(dynapiInstance.OnRestart)
	c.OnFinalShutdown(dynapiInstance.OnFinalShutdown)
	return nil
}

func dynapiParse(c *caddy.Controller) (string, time.Duration, error) {
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
