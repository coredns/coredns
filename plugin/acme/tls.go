package acme

import (
	"crypto/tls"

	"github.com/coredns/coredns/core/dnsserver"
)

func configureTLS(a ACME, zone string, conf *dnsserver.Config) error {
	err := a.GetCert(zone)
	if err != nil {
		return err
	}
	cert, err := a.Config.CacheManagedCertificate(zone)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert.Certificate}}
	tlsConfig.ClientAuth = tls.NoClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs

	setTLSDefaults(tlsConfig)

	conf.TLSConfig = tlsConfig
	return nil
}
