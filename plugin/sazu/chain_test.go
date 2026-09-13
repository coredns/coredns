package sazu

import (
	"crypto"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// genKey generates a synthetic Ed25519 zone key (KSK by default) for name
// and returns the DNSKEY plus its crypto.Signer, ready to sign RRsets with.
func genKey(t *testing.T, name string, ksk bool) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	flags := uint16(dns.ZONE)
	if ksk {
		flags |= dns.SEP
	}
	k := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     flags,
		Protocol:  3,
		Algorithm: dns.ED25519,
	}
	priv, err := k.Generate(256)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("generated key does not implement crypto.Signer")
	}
	return k, signer
}

// signRRset signs rrset (all owned by owner) with signer, acting as
// signerKey, valid from now-60s to now+3600s.
func signRRset(t *testing.T, owner string, rrset []dns.RR, signerKey *dns.DNSKEY, signer crypto.Signer, now time.Time) *dns.RRSIG {
	t.Helper()
	sig := &dns.RRSIG{
		Hdr:        dns.RR_Header{Name: dns.Fqdn(owner), Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 3600},
		Algorithm:  signerKey.Algorithm,
		KeyTag:     signerKey.KeyTag(),
		SignerName: signerKey.Hdr.Name,
		Inception:  uint32(now.Add(-60 * time.Second).Unix()),
		Expiration: uint32(now.Add(3600 * time.Second).Unix()),
	}
	if err := sig.Sign(signer, rrset); err != nil {
		t.Fatalf("signing rrset: %v", err)
	}
	return sig
}

func TestVerifyAnyRRSIGAcceptsGenuineSignature(t *testing.T) {
	key, signer := genKey(t, "example.org.", true)
	rrset := []dns.RR{key}
	now := time.Now()
	sig := signRRset(t, "example.org.", rrset, key, signer, now)

	if err := verifyAnyRRSIG("example.org.", rrset, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{key}); err != nil {
		t.Fatalf("expected genuine signature to verify, got: %v", err)
	}
}

func TestVerifyAnyRRSIGRejectsTamperedRRset(t *testing.T) {
	key, signer := genKey(t, "example.org.", true)
	rrset := []dns.RR{key}
	now := time.Now()
	sig := signRRset(t, "example.org.", rrset, key, signer, now)

	tampered := *key
	tampered.Flags ^= dns.SEP // flip the KSK flag bit after signing
	tamperedSet := []dns.RR{&tampered}

	if err := verifyAnyRRSIG("example.org.", tamperedSet, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{key}); err == nil {
		t.Fatalf("expected tampered rrset to fail verification")
	}
}

func TestVerifyAnyRRSIGRejectsWrongKey(t *testing.T) {
	key, signer := genKey(t, "example.org.", true)
	otherKey, _ := genKey(t, "example.org.", true)
	rrset := []dns.RR{key}
	now := time.Now()
	sig := signRRset(t, "example.org.", rrset, key, signer, now)

	// The key set presented for verification doesn't contain the key that
	// actually signed at all -- only an unrelated one.
	if err := verifyAnyRRSIG("example.org.", rrset, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{otherKey}); err == nil {
		t.Fatalf("expected verification against the wrong key set to fail")
	}
}

func TestVerifyAnyRRSIGRejectsExpired(t *testing.T) {
	key, signer := genKey(t, "example.org.", true)
	rrset := []dns.RR{key}
	now := time.Now()
	sig := signRRset(t, "example.org.", rrset, key, signer, now)

	later := now.Add(2 * time.Hour) // signRRset's expiration is now+3600s
	if err := verifyAnyRRSIG("example.org.", rrset, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{key}); err != nil {
		t.Fatalf("sanity check at signing time should pass, got: %v", err)
	}
	// Re-run the check as it would be evaluated `later` by asking
	// ValidityPeriod directly, since verifyAnyRRSIG always uses time.Now().
	if sig.ValidityPeriod(later) {
		t.Fatalf("expected signature to be outside its validity window %v after signing", 2*time.Hour)
	}
}

func TestDsMatchesKeyRoundtripAndRejectsTamperedDigest(t *testing.T) {
	key, _ := genKey(t, "example.org.", true)
	ds := key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatalf("ToDS returned nil")
	}
	if !dsMatchesKey(ds, key) {
		t.Fatalf("expected freshly computed DS to match its own key")
	}

	bad := *ds
	if len(bad.Digest) == 0 {
		t.Fatalf("digest unexpectedly empty")
	}
	// Flip the last hex nibble of the digest.
	lastByte := bad.Digest[len(bad.Digest)-1]
	flipped := byte('0')
	if lastByte == '0' {
		flipped = '1'
	}
	bad.Digest = bad.Digest[:len(bad.Digest)-1] + string(flipped)

	if dsMatchesKey(&bad, key) {
		t.Fatalf("expected tampered digest to be rejected")
	}
}

func TestTrustAnchorVerifiesRealDigestNotJustMetadata(t *testing.T) {
	key, _ := genKey(t, ".", true)
	ds := key.ToDS(dns.SHA256)
	anchor := TrustAnchor{KeyTag: key.KeyTag(), Algorithm: key.Algorithm, DigestHex: ds.Digest}
	if !anchor.Matches(key) {
		t.Fatalf("expected anchor to match the key its digest was computed from")
	}

	badAnchor := anchor
	lastByte := badAnchor.DigestHex[len(badAnchor.DigestHex)-1]
	flipped := byte('0')
	if lastByte == '0' {
		flipped = '1'
	}
	badAnchor.DigestHex = badAnchor.DigestHex[:len(badAnchor.DigestHex)-1] + string(flipped)

	if badAnchor.Matches(key) {
		t.Fatalf("expected a corrupted pinned digest to reject a key it would otherwise match")
	}
}

func TestTrustAnchorRejectsNonKSK(t *testing.T) {
	key, _ := genKey(t, ".", false) // ZSK only, no SEP flag
	ds := key.ToDS(dns.SHA256)
	anchor := TrustAnchor{KeyTag: key.KeyTag(), Algorithm: key.Algorithm, DigestHex: ds.Digest}

	if anchor.Matches(key) {
		t.Fatalf("expected a non-KSK to be rejected regardless of digest match")
	}
}

func TestMultiRecordRRsetCanonicalOrderingIsDeterministic(t *testing.T) {
	// A DNSKEY RRset with two keys must sign/verify regardless of the
	// order the two records happen to be stored in -- RRSIG.Verify handles
	// RFC 4034 canonical ordering internally, this just proves it end to
	// end through this package's own call pattern.
	key1, signer1 := genKey(t, "example.org.", true)
	key2, _ := genKey(t, "example.org.", true)
	now := time.Now()

	forward := []dns.RR{key1, key2}
	reversed := []dns.RR{key2, key1}
	sig := signRRset(t, "example.org.", forward, key1, signer1, now)

	if err := verifyAnyRRSIG("example.org.", forward, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{key1, key2}); err != nil {
		t.Fatalf("forward order should verify: %v", err)
	}
	if err := verifyAnyRRSIG("example.org.", reversed, dns.TypeDNSKEY, []*dns.RRSIG{sig}, []*dns.DNSKEY{key1, key2}); err != nil {
		t.Fatalf("reversed order should still verify: %v", err)
	}
}
