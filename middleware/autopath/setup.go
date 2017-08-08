package autopath

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("autopath", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})

}

func setup(c *caddy.Controller) error {
	ap, err := autoPathParse(c)
	if err != nil {
		return middleware.Error("autopath", err)
	}

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		ap.Next = next
		return ap
	})

	return nil
}

func autoPathParse(c *caddy.Controller) (AutoPath, error) {
	return AutoPath{search: []string{"default.svc.cluster.local.", "svc.cluster.local.", "cluster.local.", "com.", ""}}, nil
}
