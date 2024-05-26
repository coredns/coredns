package rlc

import (
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
)

func init() { plugin.Register("rlc", setupRlc) }

func setupRlc(c *caddy.Controller) error {
	rlc, err := rlcParse(c)
	if err != nil {
		return plugin.Error("rlc", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		rlc.Next = next
		return rlc
	})

	c.OnStartup(func() error {
		m := dnsserver.GetConfig(c).Handler("prometheus")
		if m != nil {
			rlc.metrics = m.(*metrics.Metrics)
		}
		return rlc.OnStartup()
	})
	c.OnRestart(rlc.OnReload)
	/*
		c.OnFinalShutdown(rlc.OnFinalShutdown)
		c.OnRestartFailed(rlc.OnStartup)
	*/
	return nil
}

func rlcParse(c *caddy.Controller) (*RlcHandler, error) {
	var err error
	handler := newRlcHander()

	for c.Next() {
		for c.NextBlock() {
			switch c.Val() {
			case "ttl":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				d, err := strconv.Atoi(args[0])
				if err != nil {
					return handler, err
				}
				handler.TTL = time.Duration(d) * time.Second
			case "capacity":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}
				handler.Capacity, err = strconv.Atoi(args[0])
				if err != nil {
					return handler, err
				}
			case "groupcache":
				handler.UseGroupcache = true
				args := c.RemainingArgs()
				if len(args) == 0 {
					return handler, c.ArgErr()
				}

				handler.staticPeers = strings.Split(args[len(args)-1], ",")
				handler.UseK8s = handler.staticPeers[0] == "k8s"

				if len(args) == 2 {
					i, err := strconv.Atoi(args[0])
					if err != nil {
						return handler, err
					}
					handler.CachePort = uint32(i)
				}

			case "remote":
				handler.RemoteEnabled = true
				args := c.RemainingArgs()
				if len(args) == 0 {
					continue
				}
				i, err := strconv.Atoi(args[0])
				if err != nil {
					return handler, err
				}
				handler.RemotePort = uint32(i)

			default:
				return handler, c.ArgErr()
			}

		}
	}

	return handler, err
}
