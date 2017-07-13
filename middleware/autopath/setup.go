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
	ap, err := autopathParse(c)
	if err != nil {
		return middleware.Error("autopath", err)
	}

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		ap.Next = next
		return ap
	})

	return nil
}

func autopathParse(c *caddy.Controller) (AutoPath, error) {
	ap := AutoPath{}
	ap.Origins = []string{"."}
	ap.SearchPath = []string{"members.linode.com.", "nl."}
	for c.Next() {
	}
	return ap, nil
}

var chaosVersion string
