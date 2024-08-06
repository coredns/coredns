package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/caddyserver/certmagic"
)

// CertManager takes care of obtaining and renewing TLS certificates
type CertManager struct {
	Config *certmagic.Config
	Issuer *certmagic.ACMEIssuer
	Domain string
}

func newConfig(path string) *certmagic.Config {
	acmeConfig := certmagic.NewDefault()
	acmeConfig.RenewalWindowRatio = 0.7
	acmeConfig.Issuers = []certmagic.Issuer{}
	acmeConfig.Storage = &certmagic.FileStorage{
		Path: path,
	}
	return acmeConfig
}

func newIssuer(config *certmagic.Config, ca string, email string, pool *x509.CertPool, solver *certmagic.DNS01Solver) *certmagic.ACMEIssuer {
	certmagic.DefaultACME.Email = email
	acmeIssuerTemplate := certmagic.ACMEIssuer{
		Agreed:                  true,
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
		CA:                      ca,
		TestCA:                  ca,
		Email:                   email,
		DNS01Solver:             solver,
		TrustedRoots:            pool,
	}

	acmeIssuer := certmagic.NewACMEIssuer(config, acmeIssuerTemplate)
	config.Issuers = append(config.Issuers, acmeIssuer)

	return acmeIssuer
}

func setupCertPool(caCert string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	if caCert != "" {
		certbytes, err := os.ReadFile(caCert)
		if err != nil {
			return nil, err
		}
		pemcert, _ := pem.Decode(certbytes)
		if pemcert == nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(pemcert.Bytes)
		if err != nil {
			return nil, err
		}
		pool.AddCert(cert)
	}
	return pool, nil
}

func newCertManager(domain string, config *certmagic.Config, issuer *certmagic.ACMEIssuer) *CertManager {
	return &CertManager{
		Config: config,
		Issuer: issuer,
		Domain: domain,
	}
}

func (c *CertManager) configureTLSwithACME(ctx context.Context) (*tls.Config, *certmagic.Certificate, error) {
	var cert certmagic.Certificate
	var err error

	// try loading existing certificate
	cert, err = c.cacheCertificate(ctx, c.Domain)
	if err != nil {
		log.Info("Obtaining TLS Certificate, may take a moment")
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		err = c.obtainCert(c.Domain)
		if err != nil {
			return nil, nil, err
		}
		cert, err = c.cacheCertificate(ctx, c.Domain)
		if err != nil {
			return nil, nil, err
		}
	}

	// check if renewal is required
	if cert.NeedsRenewal(c.Config) {
		log.Info("Renewing TLS Certificate")
		var err error
		err = c.renewCert(ctx, c.Domain)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: renewing certificate: %w", c.Domain, err)
		}
		// successful renewal, so update in-memory cache
		cert, err = c.cacheCertificate(ctx, c.Domain)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: reloading renewed certificate into memory: %v", c.Domain, err)
		}
	}

	// check again, if it still needs renewal something went wrong
	if cert.NeedsRenewal(c.Config) {
		log.Error("Failed to renew certificate")
	}

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert.Certificate}}
	tlsConfig.ClientAuth = tls.NoClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs

	return tlsConfig, &cert, nil
}

func (c *CertManager) obtainCert(domain string) error {
	err := c.Config.ObtainCertSync(context.Background(), domain)
	return err
}

func (c *CertManager) renewCert(ctx context.Context, domain string) error {
	log.Info("renewCert")
	err := c.Config.RenewCertSync(ctx, domain, false)
	return err
}

func (c *CertManager) cacheCertificate(ctx context.Context, domain string) (certmagic.Certificate, error) {
	cert, err := c.Config.CacheManagedCertificate(ctx, domain)
	return cert, err
}
