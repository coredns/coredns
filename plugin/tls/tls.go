package tls

import (
	ctls "crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/tls"
)

var log = clog.NewWithPlugin("tls")

func init() {
	plugin.Register("tls", setup)
	caddy.RegisterEventHook("tls-acme", acmeStartupHook)
}

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
		if len(args) == 0 {
			tlsConfig, err := parseACMETLS(c, config)
			if err != nil {
				return err
			}
			config.TLSConfig = tlsConfig
			continue
		}
		if err := parseManualTLS(c, config, args); err != nil {
			return err
		}
	}
	return nil
}

func parseManualTLS(c *caddy.Controller, config *dnsserver.Config, args []string) error {
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
		absKeyLog, err := filepath.Abs(keyLog)
		if err != nil {
			return c.Errf("unable to write TLS Key Log to %q: %s", keyLog, err)
		}
		f, err := os.OpenFile(absKeyLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return c.Errf("unable to write TLS Key Log to %q: %s", absKeyLog, err)
		}
		c.OnShutdown(func() error {
			f.Close()
			return nil
		})
		tls.KeyLogWriter = f
		log.Warningf("Writing TLS Key Log to %q\n", absKeyLog)
	}

	config.TLSConfig = tls
	return nil
}

func parseACMETLS(c *caddy.Controller, config *dnsserver.Config) (*ctls.Config, error) {
	options, err := defaultACMEOptions(config.Root)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for c.NextBlock() {
		name := c.Val()
		args := c.RemainingArgs()
		if seen[name] {
			return nil, c.Errf("ACME option %q can only be specified once", name)
		}
		seen[name] = true
		switch name {
		case "acme":
			if len(args) == 0 {
				return nil, c.ArgErr()
			}
			options.domains = args
		case "email":
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			options.email = args[0]
		case "ca":
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			options.ca = args[0]
		case "storage":
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			options.storage, err = resolveACMEPath(config.Root, args[0])
			if err != nil {
				return nil, err
			}
		case "ca_root":
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			options.caRoot, err = resolveACMEPath(config.Root, args[0])
			if err != nil {
				return nil, err
			}
		case "resolver":
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			options.resolver = args[0]
		default:
			return nil, c.Errf("unknown ACME option %q", name)
		}
	}
	if err := validateACMEOptions(&options); err != nil {
		return nil, err
	}

	runtime := getACMERuntime(c)
	entry, err := runtime.add(options)
	if err != nil {
		return nil, err
	}
	runtime.installChallengeHandlers(c)
	return entry.tlsConfig(), nil
}

func resolveACMEPath(root, path string) (string, error) {
	if !filepath.IsAbs(path) && root != "" {
		path = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving ACME path %q: %w", path, err)
	}
	return abs, nil
}
