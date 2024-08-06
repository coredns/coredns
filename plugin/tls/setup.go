package tls

import (
	"context"
	ctls "crypto/tls"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/tls"

	"github.com/caddyserver/certmagic"
)

func init() { plugin.Register("tls", setup) }

func setup(c *caddy.Controller) error {
	err := parseTLS(c)
	if err != nil {
		return plugin.Error("tls", err)
	}
	return nil
}

var (
	log            = clog.NewWithPlugin("tls")
	r              = renewCert{quit: make(chan bool), renew: make(chan bool)}
	once, shutOnce sync.Once
)

func parseTLS(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	if config.TLSConfig != nil {
		return plugin.Error("tls", c.Errf("TLS already configured for this server instance"))
	}

	for c.Next() {
		args := c.RemainingArgs()

		if args[0] == "acme" {
			log.Debug("Starting ACME Setup")

			var err error
			var email string
			var domain string
			var dnsProvider string
			caServer := certmagic.LetsEncryptStagingCA
			var caCert string
			checkInterval := 60
			certPath := "./.tls/"

			for c.NextBlock() {
				token := c.Val()
				switch token {
				case "email":
					emailArgs := c.RemainingArgs()
					if len(emailArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to email"))
					}
					email = emailArgs[0]
				case "domain":
					domainArgs := c.RemainingArgs()
					if len(domainArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to domain"))
					}
					domain = domainArgs[0]
				case "checkInterval":
					checkIntervalArgs := c.RemainingArgs()
					if len(checkIntervalArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to checkInterval"))
					}
					checkInterval, err = strconv.Atoi(checkIntervalArgs[0])
					if err != nil {
						return plugin.Error("tls", c.Errf("checkIntervalArgs needs to be a number"))
					}
				case "dnsProvider":
					dnsProviderArgs := c.RemainingArgs()
					if len(dnsProviderArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to dnsProvider"))
					}
					dnsProvider = dnsProviderArgs[0]
				case "certPath":
					certPathArgs := c.RemainingArgs()
					if len(certPathArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to certPath"))
					}
					certPath = certPathArgs[0]
				case "caServer":
					caServerArgs := c.RemainingArgs()
					if len(caServerArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to caServer"))
					}
					caServer = caServerArgs[0]
				default:
					return c.Errf("unknown argument to acme '%s'", token)
				}
			}

			dnsSolver := createDNSSolver(dnsProvider)
			pool, err := setupCertPool(caCert)
			if err != nil {
				log.Errorf("Failed to setup certificate pool: %v\n", err)
			}

			certmagicConfig := newConfig(certPath)
			certmagicIssuer := newIssuer(certmagicConfig, caServer, email, pool, dnsSolver)
			certManager := newCertManager(domain, certmagicConfig, certmagicIssuer)

			ctx := context.Background()

			tlsconf, cert, err := certManager.configureTLSwithACME(ctx)
			if err != nil {
				log.Errorf("Failed to setup TLS automatically: %v \n", err)
			}
			config.TLSConfig = tlsconf

			once.Do(func() {
				go func() {
					log.Debug("Starting certificate renewal loop in the background")
					for {
						time.Sleep(time.Duration(checkInterval) * time.Minute)
						if cert.NeedsRenewal(certManager.Config) {
							log.Info("Certificate expiring soon, initializing reload")
							r.renew <- true
						}
					}
				}()
				caddy.RegisterEventHook("updateCert", hook)
			})
			shutOnce.Do(func() {
				c.OnFinalShutdown(func() error {
					log.Debug("Quiting renewal checker")
					r.quit <- true
					return nil
				})
			})
		} else {
			if len(args) < 2 || len(args) > 3 {
				return plugin.Error("tls", c.ArgErr())
			}
			clientAuth := ctls.NoClientCert
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

			config.TLSConfig = tls
		}
	}
	return nil
}
