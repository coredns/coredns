package acme

import (
	"crypto/tls"
	"net"
	"strconv"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
)

const pluginName = "acme"

func init() {
	plugin.Register(pluginName, setup)
}

func setup(c *caddy.Controller) error {
	acmeTemplate, zoneName, err := parseACME(c)
	provider := Provider{
		recordMap: make(map[string]*RecordStore),
	}
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	config := dnsserver.GetConfig(c)
	acmeConfig := AcmeConfig{
		Zone: zoneName,
	}
	acmeHandler := &AcmeHandler{
		provider:   &provider,
		AcmeConfig: &acmeConfig,
	}
	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		acmeHandler.Next = next
		return acmeHandler
	})
	c.OnFirstStartup(func() error {
		go func() error {
			authoritativeNameservers, err := getAuthoritativeNameServers(zoneName)
			if err != nil {
				return err
			}
			authoritativeNameserver := authoritativeNameservers[len(authoritativeNameservers)-1]

			ipAddr, err := getExternalIpAddress(authoritativeNameserver)
			if err != nil {
				log.Error(err)
				return err
			}
			acmeHandler.Ipv4Addr = net.ParseIP(ipAddr).To4()
			acmeHandler.Ipv6Addr = net.ParseIP(ipAddr).To16()
			acmeHandler.AuthoritativeNameserver = authoritativeNameserver

			acmeTemplate.DNS01Solver = &certmagic.DNS01Solver{
				DNSProvider: &provider,
				Resolvers:   []string{ipAddr},
			}

			A := NewACME(acmeTemplate, zoneName)
			err = A.IssueCert([]string{zoneName})
			if err != nil {
				log.Error(err)
				return err
			}
			log.Info("Certificate Issued")
			err = configureTLS(A, zoneName, config)
			if err != nil {
				log.Error(err)
				return err
			}
			log.Info("TLS Configured")
			return nil
		}()
		return nil
	})
	return nil
}

func setTLSDefaults(tlsConfig *tls.Config) {
	tlsConfig.MinVersion = tls.VersionTLS12
	tlsConfig.MaxVersion = tls.VersionTLS13
	tlsConfig.CipherSuites = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}
	tlsConfig.PreferServerCipherSuites = true
}

func parseACME(c *caddy.Controller) (certmagic.ACMEManager, string, error) {
	acmeTemplate := certmagic.ACMEManager{
		Agreed:                  true,
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
	}
	var zoneName string
	for c.Next() {
		for c.NextBlock() {
			term := strings.ToLower(c.Val())
			switch term {
			case DOMAIN:
				args := c.RemainingArgs()
				if len(args) > 1 {
					return acmeTemplate, zoneName, c.Errf("unexpected number of arguments: %#v", args)
				}
				zoneName = args[0]
			case CHALLENGE:
				args := c.RemainingArgs()
				challenge := args[0]
				if !(len(args) == 3 && args[1] == PORT) {
					return acmeTemplate, zoneName, c.Errf("unexpected number of arguments: %#v", args)
				}
				port, err := strconv.Atoi(args[2])
				if err != nil {
					return acmeTemplate, zoneName, c.Errf("%s port is not an int: %#v", challenge, args)
				}
				switch challenge {
				case HTTPChallenge:
					acmeTemplate.AltHTTPPort = port
					acmeTemplate.DisableHTTPChallenge = false
				case TLPSALPNChallenge:
					acmeTemplate.AltTLSALPNPort = port
					acmeTemplate.DisableTLSALPNChallenge = false
				default:
					return acmeTemplate, zoneName, c.Errf("unexpected challenge %s: challenge should only be tlsalpn or http", challenge)
				}
			default:
				return acmeTemplate, zoneName, c.Errf("unexpected term: %s: term should only be challenge or domain", term)
			}
		}
	}
	if zoneName == "" {
		return acmeTemplate, zoneName, c.Errf("Domain not provided")
	}
	acmeTemplate.CA = certmagic.LetsEncryptProductionCA
	return acmeTemplate, zoneName, nil
}
