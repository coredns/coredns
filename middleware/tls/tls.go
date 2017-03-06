package root

import (
	"github.com/coredns/coredns/core/dnsserver"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("tls", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	// no options yet
	config.TLSConfig = "yo"

	return nil
}
