// Package sazu implements SAZU (Self-Authenticated Zone Update): a
// split-signing DNSSEC scheme where a customer's own signer holds the
// private key, and this server only ever accepts already-signed zone
// updates, authenticated via SIG(0) (RFC 2931) carried on RFC 2136 dynamic
// UPDATE messages. See the design document for the full protocol.
//
// This package is not yet registered in plugin.cfg -- it is being built
// and tested as a standalone library first (chain-of-trust validation,
// SIG(0) handling), the same staged approach used for the earlier Rust/
// rDNS port, before being wired into CoreDNS's request-handling chain.
package sazu

import (
	"strings"

	"github.com/miekg/dns"
)

// TrustAnchor pins one root-zone KSK by its RFC 4034 §5.1.4 digest -- the
// same construction a DS record uses, just carried here as a hex string
// because the root has no parent to publish one. This is the base case
// chain-of-trust validation bootstraps from.
type TrustAnchor struct {
	KeyTag    uint16
	Algorithm uint8
	// DigestHex is the SHA-256 digest of hash(root name "." || DNSKEY
	// RDATA), hex-encoded.
	DigestHex string
}

// RootTrustAnchors are the current IANA root zone KSKs.
func RootTrustAnchors() []TrustAnchor {
	return []TrustAnchor{
		{
			KeyTag:    20326,
			Algorithm: dns.RSASHA256,
			DigestHex: "E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D",
		},
	}
}

// Matches reports whether key is the KSK this trust anchor pins. It
// verifies the real SHA-256 digest of the key -- not just key tag,
// algorithm, and the SEP (KSK) flag, which alone would accept any key an
// attacker chose with the right metadata but different key material.
func (a TrustAnchor) Matches(key *dns.DNSKEY) bool {
	if key == nil {
		return false
	}
	if key.KeyTag() != a.KeyTag || key.Algorithm != a.Algorithm || key.Flags&dns.SEP == 0 {
		return false
	}
	ds := key.ToDS(dns.SHA256)
	if ds == nil {
		return false
	}
	return strings.EqualFold(ds.Digest, a.DigestHex)
}
