package autopath

import (
	"fmt"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/chaos"

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

	c.OnStartup(func() error {
		// So we know for sure this is initialized.
		ch := dnsserver.GetMiddleware(c, "chaos")
		if ch != nil {
			if ch, ok := ch.(chaos.Chaos); ok {
				fmt.Printf("%v\n", ch.AutoPath())
			}
		}
		return nil
	})

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		ap.Next = next
		return ap
	})

	return nil
}

func autoPathParse(c *caddy.Controller) (AutoPath, error) {
	return AutoPath{search: []string{"default.svc.cluster.local.", "svc.cluster.local.", "cluster.local.", "com.", ""}}, nil
}

//
