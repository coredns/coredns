package tls

import (
	"context"
	ctls "crypto/tls"
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

const (
	defaultCA            = "https://acme-v02.api.letsencrypt.org/directory"
	defaultEmail         = "doesnotexist@test.com"
	defaultCheckInterval = 15
	defaultPort          = 53
	defaultCertPath      = "./.tls/"
)

func parseTLS(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	var tlsconf *ctls.Config
	var cert *certmagic.Certificate
	var err error
	clientAuth := ctls.NoClientCert

	if config.TLSConfig != nil {
		return plugin.Error("tls", c.Errf("TLS already configured for this server instance"))
	}
	for c.Next() {
		args := c.RemainingArgs()

		if args[0] == "acme" {
			log.Debug("Starting ACME Setup")

			ctx := context.Background()

			var domainNameACME string
			var caCert string
			port := defaultPort
			email := defaultEmail
			ca := defaultCA
			checkInterval := defaultCheckInterval
			certPath := defaultCertPath

			for c.NextBlock() {
				token := c.Val()
				switch token {
				case "domain":
					domainArgs := c.RemainingArgs()
					if len(domainArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to domain"))
					}
					domainNameACME = domainArgs[0]
				case "ca":
					caArgs := c.RemainingArgs()
					if len(caArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to ca"))
					}
					ca = caArgs[0]
				case "cacert":
					caCertArgs := c.RemainingArgs()
					if len(caCertArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to cacert"))
					}
					caCert = caCertArgs[0]
				case "email":
					emailArgs := c.RemainingArgs()
					if len(emailArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to email"))
					}
					email = emailArgs[0]
				case "port":
					portArgs := c.RemainingArgs()
					if len(portArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to port"))
					}
					port, err = strconv.Atoi(portArgs[0])
					if err != nil {
						log.Errorf("Failed to convert port argument to integer: %v \n", err)
					}
				case "certpath":
					certPathArgs := c.RemainingArgs()
					if len(certPathArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to certpath"))
					}
					certPath = certPathArgs[0]
				case "checkinterval":
					checkIntervalArgs := c.RemainingArgs()
					if len(checkIntervalArgs) > 1 {
						return plugin.Error("tls", c.Errf("Too many arguments to checkinterval"))
					}
					interval, err := strconv.Atoi(checkIntervalArgs[0])
					if err != nil {
						return plugin.Error("Failed to convert checkInterval argument to integer: %v \n", err)
					}
					checkInterval = interval
				default:
					return c.Errf("unknown argument to acme '%s'", token)
				}
			}

			pool, err := setupCertPool(caCert)
			if err != nil {
				log.Errorf("Failed to add the custom CA certfiicate to the pool of trusted certificates: %v, \n", err)
			}
			solver := newDNSSolver(port)
			certmagicConfig := newConfig(certPath)
			certmagicIssuer := newIssuer(certmagicConfig, ca, email, pool, solver)
			certManager := newCertManager(domainNameACME, certmagicConfig, certmagicIssuer)

			tlsconf, cert, err = certManager.configureTLSwithACME(ctx)
			if err != nil {
				log.Errorf("Failed to setup TLS automatically: %v \n", err)
			}
			config.TLSConfig = tlsconf

			once.Do(func() {
				// start a loop that checks for renewals
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
			//No ACME part - plugin continues to work like the normal tls plugin
			if len(args) < 2 || len(args) > 3 {
				return plugin.Error("tls", c.ArgErr())
			}
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
			tlsconf, err = tls.NewTLSConfigFromArgs(args...)
			if err != nil {
				return err
			}
			tlsconf.ClientAuth = clientAuth
			// NewTLSConfigs only sets RootCAs, so we need to let ClientCAs refer to it.
			tlsconf.ClientCAs = tlsconf.RootCAs
			config.TLSConfig = tlsconf
		}
	}
	return nil
}
