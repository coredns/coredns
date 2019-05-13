package tls

import (
	ctls "crypto/tls"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/tls"

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

	if config.TLSConfig != nil {
		return plugin.Error("tls", c.Errf("TLS already configured for this server instance"))
	}

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) < 2 || len(args) > 3 {
			return plugin.Error("tls", c.ArgErr())
		}
		tls, err := tls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return plugin.Error("tls", err)
		}
		config.TLSConfig = tls

		// If CA is explicitly specified, we assume client authentication is needed, and adjust TLS config accordingly.
		if len(args) == 3 {
			config.TLSConfig.ClientCAs = config.TLSConfig.RootCAs
			config.TLSConfig.ClientAuth = ctls.RequireAndVerifyClientCert
		}
	}
	return nil
}
