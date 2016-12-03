package whoareyou

import (
	"github.com/miekg/coredns/core/dnsserver"
	"github.com/miekg/coredns/middleware"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("whoareyou", caddy.Plugin{
		ServerType: "dns",
		Action:     setupWhoareyou,
	})
}

func setupWhoareyou(c *caddy.Controller) error {
	c.Next() // 'whoareyou'
	if c.NextArg() {
		return middleware.Error("whoareyou", c.ArgErr())
	}

	scopes, err := getScopes()
	if err != nil {
		return middleware.Error("whoareyou", err)
	}

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		return Whoareyou{Next: next, scopes: scopes}
	})

	return nil
}
