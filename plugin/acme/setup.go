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
	"github.com/libdns/libdns"
)

const pluginName = "acme"

func init() {
	plugin.Register(pluginName, setup)
}

func setup(c *caddy.Controller) error {
	acmeTemplate, zone, err := parseACME(c)
	provider := Provider{
		recordsForZone: make(map[string][]libdns.Record),
	}
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	config := dnsserver.GetConfig(c)
	acmeConfig := AcmeConfig{
		Zone: zone,
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
			authoritativeNameservers, err := getAuthoritativeNameServers(zone)
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

			A := NewACME(acmeTemplate, zone)
			err = A.IssueCert([]string{zone})
			if err != nil {
				log.Error(err)
				return err
			}
			log.Info("Certificate Issued")
			err = configureTLS(A, zone, config)
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
	var err error
	var zone string
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 1 {
			return acmeTemplate, zone, c.Errf("Unexpected number of arguments: %#v", args)
		}
		zone = args[0]
		for c.NextBlock() {
			challenge := strings.ToLower(c.Val())
			switch challenge {
			case HTTPChallenge:
				args := c.RemainingArgs()
				if len(args) > 1 {
					return acmeTemplate, zone, c.Errf("Unexpected number of arguments: %#v", args)
				}
				httpPort := 80
				if len(args) == 1 {
					httpPort, err = strconv.Atoi(args[0])
					if err != nil {
						return acmeTemplate, zone, c.Errf("HTTP port is not an int: %#v", args)
					}
				}
				acmeTemplate.AltHTTPPort = httpPort
				acmeTemplate.DisableHTTPChallenge = false
			case TLPSALPNChallenge:
				args := c.RemainingArgs()
				if len(args) > 1 {
					return acmeTemplate, zone, c.Errf("Unexpected number of arguments: %#v", args)
				}
				var err error
				tlsAlpnPort := 443
				if len(args) == 1 {
					tlsAlpnPort, err = strconv.Atoi(args[0])
					if err != nil {
						return acmeTemplate, zone, c.Errf("TlsAlpn port is not an int: %#v", args)
					}
				}
				acmeTemplate.AltTLSALPNPort = tlsAlpnPort
				acmeTemplate.DisableTLSALPNChallenge = false
			default:
				return acmeTemplate, zone, c.Errf("Unexpected challenge: %s", challenge)
			}
		}
	}
	acmeTemplate.CA = certmagic.LetsEncryptStagingCA
	return acmeTemplate, zone, nil
}
