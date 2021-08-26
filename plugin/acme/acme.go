package acme

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/caddyserver/certmagic"
)

const (
	HTTPChallenge     = "http"
	TLPSALPNChallenge = "tlsalpn"
	CHALLENGE         = "challenge"
	DOMAIN            = "domain"
	PORT              = "port"
)

type ACME struct {
	Manager *certmagic.ACMEManager
	Config  *certmagic.Config
	Zone    string
}

func NewACME(acmeManagerTemplate certmagic.ACMEManager, zone string) ACME {
	configTemplate := certmagic.NewDefault()
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return configTemplate, nil
		},
	})
	config := certmagic.New(cache, *configTemplate)
	acmeManager := certmagic.NewACMEManager(config, acmeManagerTemplate)
	config.Issuers = append(config.Issuers, acmeManager)
	return ACME{
		Config:  config,
		Manager: acmeManager,
		Zone:    zone,
	}
}

func (a ACME) OnStartup() error {
	httpPort := fmt.Sprintf(":%d", a.Manager.AltHTTPPort)
	tlsalpnPort := fmt.Sprintf(":%d", a.Manager.AltTLSALPNPort)
	tlsConfig := a.Config.TLSConfig()
	var err error
	if !a.Manager.DisableTLSALPNChallenge {
		go func() {
			_, err = tls.Listen("tcp", tlsalpnPort, tlsConfig)
		}()
	}
	if !a.Manager.DisableHTTPChallenge {
		go func() { err = http.ListenAndServe(httpPort, a.Manager.HTTPChallengeHandler(http.NewServeMux())) }()
	}
	return err
}

func (a ACME) IssueCert(zones []string) error {
	err := a.Config.ManageSync(zones)
	return err
}

func (a ACME) GetCert(zone string) error {
	err := a.Config.ObtainCert(context.Background(), zone, false)
	return err
}

func (a ACME) RevokeCert(zone string) error {
	err := a.Config.RevokeCert(context.Background(), zone, 0, false)
	return err
}
