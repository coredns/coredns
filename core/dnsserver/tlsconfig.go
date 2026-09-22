package dnsserver

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"slices"
)

// sharedTLSConfig returns the TLS configuration for a listener shared by group.
// TLS is negotiated before CoreDNS knows which zone will handle the DNS request,
// so listener-wide identity and client-authentication settings must agree.
func sharedTLSConfig(addr string, group []*Config) (*tls.Config, error) {
	if len(group) == 0 {
		return nil, nil
	}

	first := group[0]
	if first == nil {
		return nil, fmt.Errorf("nil config for shared listener %s", addr)
	}

	for _, conf := range group[1:] {
		if conf == nil {
			return nil, fmt.Errorf("nil config for shared listener %s", addr)
		}
		if err := compatibleTLSConfig(first, conf); err != nil {
			return nil, fmt.Errorf("conflicting TLS configuration for shared listener %s between zones %q and %q: %w", addr, first.Zone, conf.Zone, err)
		}
	}

	if first.TLSConfig == nil {
		return nil, nil
	}
	return first.TLSConfig.Clone(), nil
}

func sameServerBlock(a, b *Config) bool {
	aFirst := a.firstConfigInBlock
	if aFirst == nil {
		aFirst = a
	}
	bFirst := b.firstConfigInBlock
	if bFirst == nil {
		bFirst = b
	}
	return aFirst == bFirst
}

func compatibleTLSConfig(aConfig, bConfig *Config) error {
	a := aConfig.TLSConfig
	b := bConfig.TLSConfig
	if a == nil || b == nil {
		if a == b {
			return nil
		}
		return fmt.Errorf("TLS is configured for only one server block")
	}

	if a.ClientAuth != b.ClientAuth {
		return fmt.Errorf("client authentication policies differ")
	}
	if !certPoolsEqual(a.ClientCAs, b.ClientCAs) {
		return fmt.Errorf("client CA pools differ")
	}
	if !certificatesEqual(a.Certificates, b.Certificates) {
		return fmt.Errorf("server certificates differ")
	}

	// Dynamic callbacks cannot be compared directly. Allow them only when the
	// configs came from the same server block, are the same config object, or a
	// plugin supplied a trusted identity proving the policies are equivalent.
	trustedDynamicPolicy := sameServerBlock(aConfig, bConfig) || a == b ||
		(aConfig.tlsConfigIdentity != nil && aConfig.tlsConfigIdentity == bConfig.tlsConfigIdentity)
	if !trustedDynamicPolicy && (a.GetCertificate != nil || b.GetCertificate != nil ||
		a.GetConfigForClient != nil || b.GetConfigForClient != nil ||
		a.VerifyPeerCertificate != nil || b.VerifyPeerCertificate != nil ||
		a.VerifyConnection != nil || b.VerifyConnection != nil) {
		return fmt.Errorf("dynamic TLS callbacks differ")
	}

	if a.MinVersion != b.MinVersion || a.MaxVersion != b.MaxVersion {
		return fmt.Errorf("TLS version policies differ")
	}
	if !slices.Equal(a.CipherSuites, b.CipherSuites) {
		return fmt.Errorf("cipher suite policies differ")
	}
	if !slices.Equal(a.CurvePreferences, b.CurvePreferences) {
		return fmt.Errorf("curve preference policies differ")
	}
	if a.SessionTicketsDisabled != b.SessionTicketsDisabled {
		return fmt.Errorf("session ticket policies differ")
	}

	return nil
}

func certPoolsEqual(a, b *x509.CertPool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(b)
}

func certificatesEqual(a, b []tls.Certificate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].Certificate) != len(b[i].Certificate) {
			return false
		}
		for j := range a[i].Certificate {
			if !bytes.Equal(a[i].Certificate[j], b[i].Certificate[j]) {
				return false
			}
		}
		if !bytes.Equal(a[i].OCSPStaple, b[i].OCSPStaple) {
			return false
		}
		if len(a[i].SignedCertificateTimestamps) != len(b[i].SignedCertificateTimestamps) {
			return false
		}
		for j := range a[i].SignedCertificateTimestamps {
			if !bytes.Equal(a[i].SignedCertificateTimestamps[j], b[i].SignedCertificateTimestamps[j]) {
				return false
			}
		}
	}
	return true
}
