package tls

import (
	ctls "crypto/tls"
	"os"
	"path/filepath"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/tls"
)

var log = clog.NewWithPlugin("tls")

func init() { plugin.Register("tls", setup) }

func setup(c *caddy.Controller) error {
	err := parseTLS(c)
	if err != nil {
		return plugin.Error("tls", err)
	}
	return nil
}

func parseTLS(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	if config.TLSConfig != nil {
		return plugin.Error("tls", c.Errf("TLS already configured for this server instance"))
	}

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) < 2 || len(args) > 3 {
			return plugin.Error("tls", c.ArgErr())
		}
		clientAuth := ctls.NoClientCert
		var keyLog string
		for c.NextBlock() {
			switch c.Val() {
			case "client_auth":
				authTypeArgs := c.RemainingArgs()
				if len(authTypeArgs) != 1 {
					return c.ArgErr()
				}
				switch authTypeArgs[0] {
				case "nocert":
					clientAuth = ctls.NoClientCert
				case "request":
					clientAuth = ctls.RequestClientCert
				case "require":
					clientAuth = ctls.RequireAnyClientCert
				case "verify_if_given":
					clientAuth = ctls.VerifyClientCertIfGiven
				case "require_and_verify":
					clientAuth = ctls.RequireAndVerifyClientCert
				default:
					return c.Errf("unknown authentication type '%s'", authTypeArgs[0])
				}
			case "keylog":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return c.ArgErr()
				}
				keyLog = args[0]
				if !filepath.IsAbs(keyLog) && config.Root != "" {
					keyLog = filepath.Join(config.Root, keyLog)
				}
			default:
				return c.Errf("unknown option '%s'", c.Val())
			}
		}
		for i := range args {
			if !filepath.IsAbs(args[i]) && config.Root != "" {
				args[i] = filepath.Join(config.Root, args[i])
			}
		}
		tls, err := tls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		tls.ClientAuth = clientAuth
		// NewTLSConfigFromArgs only sets RootCAs, so we need to let ClientCAs refer to it.
		tls.ClientCAs = tls.RootCAs

		if len(keyLog) > 0 {
			writer, err := os.OpenFile(keyLog, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return c.Errf("unable to write TLS Key Log: %s", err)
			}
			tls.KeyLogWriter = writer
			keyLog, _ = filepath.Abs(keyLog)
			log.Infof("Writing TLS Key Log to %q\n", keyLog)
		}

		config.TLSConfig = tls
	}
	return nil
}
