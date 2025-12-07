package proxyproto

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/pires/go-proxyproto"
)

func init() { plugin.Register("proxyproto", setup) }

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)
	if config.ProxyProtoConnPolicy != nil {
		return plugin.Error("proxyproto", errors.New("proxy protocol already configured for this server instance"))
	}
	var (
		allowedIPNets []*net.IPNet
		policy        = proxyproto.IGNORE
	)
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 0 {
			return plugin.Error("proxyproto", c.ArgErr())
		}
		for c.NextBlock() {
			switch c.Val() {
			case "allow":
				for _, v := range c.RemainingArgs() {
					_, ipnet, err := net.ParseCIDR(v)
					if err != nil {
						return plugin.Error("proxyproto", fmt.Errorf("%s: %w", v, err))
					}
					allowedIPNets = append(allowedIPNets, ipnet)
				}
			case "default":
				v := c.RemainingArgs()
				if len(v) != 1 {
					return plugin.Error("proxyproto", c.ArgErr())
				}
				switch strings.ToLower(v[0]) {
				case "use":
					policy = proxyproto.USE
				case "ignore":
					policy = proxyproto.IGNORE
				case "reject":
					policy = proxyproto.REJECT
				case "skip":
					policy = proxyproto.SKIP
				default:
					return plugin.Error("proxyproto", c.ArgErr())
				}
			default:
				return c.Errf("unknown option '%s'", c.Val())
			}
		}
	}
	config.ProxyProtoConnPolicy = func(connPolicyOptions proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
		if len(allowedIPNets) == 0 {
			return proxyproto.USE, nil
		}
		h, _, _ := net.SplitHostPort(connPolicyOptions.Upstream.String())
		ip := net.ParseIP(h)
		if ip == nil {
			return proxyproto.REJECT, nil
		}
		for _, ipnet := range allowedIPNets {
			if ipnet.Contains(ip) {
				return proxyproto.USE, nil
			}
		}
		return policy, nil
	}
	return nil
}
