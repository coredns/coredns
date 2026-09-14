package sazu

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/miekg/dns"
)

// GenerateEd25519Key creates a fresh Ed25519 zone key for owner, suitable
// for both SIG(0) transaction signing and DNSSEC -- SAZU's central design
// decision (the design doc's §9.1): one key does both jobs, so there is
// never a separate key-placement step. Ed25519 is the default here for the
// same reason the earlier Rust/rDNS client tooling defaulted to it: it's
// what Go's stdlib (like `ring` on the Rust side) can generate and sign
// with directly, no external key material or ASN.1 wrangling required.
func GenerateEd25519Key(owner string, ksk bool) (*dns.DNSKEY, ed25519.PrivateKey, error) {
	k := newDNSKEY(owner, ksk)
	priv, err := k.Generate(256)
	if err != nil {
		return nil, nil, err
	}
	edpriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected private key type %T", priv)
	}
	return k, edpriv, nil
}

func newDNSKEY(owner string, ksk bool) *dns.DNSKEY {
	flags := uint16(dns.ZONE)
	if ksk {
		flags |= dns.SEP
	}
	return &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(owner), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     flags,
		Protocol:  3,
		Algorithm: dns.ED25519,
	}
}

// SavePrivateKey writes priv to path in BIND9's private-key-file format
// (Private-key-format: v1.3) via DNSKEY.PrivateKeyString -- the same
// format dnssec-keygen/named produce, so a SAZU key can also be inspected
// or used with standard DNSSEC tooling, not just this package's own tools.
func SavePrivateKey(path string, key *dns.DNSKEY, priv ed25519.PrivateKey) error {
	return os.WriteFile(path, []byte(key.PrivateKeyString(priv)), 0o600)
}

// SaveEncryptedPrivateKey writes priv to path exactly as SavePrivateKey
// would, except encrypted at rest with a key derived from passphrase --
// §10.8's key custody hardening (see keycrypt.go). The file is no longer
// directly readable by standard DNSSEC tooling; decrypt it back to plain
// BIND format first (LoadPrivateKey with the same passphrase, then
// SavePrivateKey) if that's ever needed.
func SaveEncryptedPrivateKey(path string, key *dns.DNSKEY, priv ed25519.PrivateKey, passphrase []byte) error {
	enc, err := encryptKeyFileBytes([]byte(key.PrivateKeyString(priv)), passphrase)
	if err != nil {
		return fmt.Errorf("encrypting %s: %w", path, err)
	}
	return os.WriteFile(path, enc, 0o600)
}

// LoadPrivateKey reads back a key file written by SavePrivateKey or
// SaveEncryptedPrivateKey, transparently telling the two apart. passphrase
// is only used (and only needed) for an encrypted file; pass nil for a
// plain one. Ed25519 only, matching the scope of the client tooling built
// on top of it -- miekg/dns can write BIND's private-key-file format for
// RSA/ECDSA too, but has no built-in reader for any algorithm, so a full
// reader is out of scope until something other than this package's own
// Ed25519 keys needs reading back.
func LoadPrivateKey(path string, passphrase []byte) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isEncryptedKeyFile(data) {
		if len(passphrase) == 0 {
			return nil, fmt.Errorf("%s: this key file is encrypted -- a passphrase is required", path)
		}
		data, err = decryptKeyFileBytes(data, passphrase)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(line, "PrivateKey: ")
		if !ok {
			continue
		}
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("%s: decoding private key seed: %w", path, err)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("%s: unexpected seed length %d (want %d) -- not an Ed25519 key file?",
				path, len(seed), ed25519.SeedSize)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	return nil, fmt.Errorf("%s: no PrivateKey line found", path)
}

// DNSKEYFor builds the public DNSKEY record for priv, owned by owner.
func DNSKEYFor(owner string, priv ed25519.PrivateKey, ksk bool) *dns.DNSKEY {
	k := newDNSKEY(owner, ksk)
	pub := priv.Public().(ed25519.PublicKey)
	k.PublicKey = base64.StdEncoding.EncodeToString(pub)
	return k
}

// LoadOrGenerateKey loads an existing key file at path (transparently
// handling either the plain BIND format or this package's own
// passphrase-encrypted one -- see LoadPrivateKey), or generates and saves
// a new one if it doesn't exist yet. The bool result reports whether a
// new key was generated.
//
// passphrase controls encryption, symmetrically for both directions: pass
// nil for the original, plain BIND-format behavior (reading a plain file,
// or writing one when generating); pass a non-nil passphrase to decrypt
// an existing encrypted file, or to have a newly generated key saved
// encrypted with it.
func LoadOrGenerateKey(path, owner string, ksk bool, passphrase []byte) (*dns.DNSKEY, ed25519.PrivateKey, bool, error) {
	if _, err := os.Stat(path); err == nil {
		priv, err := LoadPrivateKey(path, passphrase)
		if err != nil {
			return nil, nil, false, err
		}
		return DNSKEYFor(owner, priv, ksk), priv, false, nil
	}
	k, priv, err := GenerateEd25519Key(owner, ksk)
	if err != nil {
		return nil, nil, false, err
	}
	if passphrase != nil {
		err = SaveEncryptedPrivateKey(path, k, priv, passphrase)
	} else {
		err = SavePrivateKey(path, k, priv)
	}
	if err != nil {
		return nil, nil, false, err
	}
	return k, priv, true, nil
}
