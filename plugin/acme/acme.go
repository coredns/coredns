package acme

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/caddyserver/certmagic"
)

// Keywords to be used in parsing the Corefile
const (
	HTTPChallenge     = "http"
	TLPSALPNChallenge = "tlsalpn"
	CHALLENGE         = "challenge"
	DOMAIN            = "domain"
	PORT              = "port"
)

// ACME contains the data needed to perform the protocol
type ACME struct {
	Manager *certmagic.ACMEManager
	Config  *certmagic.Config
	Zone    string
}

func newACME(acmeManagerTemplate certmagic.ACMEManager, zone string) ACME {
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
