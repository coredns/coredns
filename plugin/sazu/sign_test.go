package sazu

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func dnskeyRRFor(key *dns.DNSKEY) *dns.DNSKEY {
	return &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: key.Hdr.Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     key.Flags,
		Protocol:  key.Protocol,
		Algorithm: key.Algorithm,
		PublicKey: key.PublicKey,
	}
}

func TestSignZoneContentProducesOneRRSIGPerRRset(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	soa := testSOA(1)
	a1 := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a2 := testA("www.example.org.", net.IPv4(203, 0, 113, 11)) // same RRset as a1
	ns := &dns.NS{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: "ns1.example.org."}

	rrs := []dns.RR{dnskeyRR, soa, a1, a2, ns}
	now := time.Now()
	signed, err := SignZoneContent(rrs, dnskeyRR, priv, now.Add(-time.Hour), now.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}

	// 5 original records (DNSKEY, SOA, 2xA in one RRset, NS) group into 4
	// RRsets (DNSKEY, SOA, A, NS), so 4 RRSIGs should be added.
	var sigCount int
	for _, rr := range signed {
		if _, ok := rr.(*dns.RRSIG); ok {
			sigCount++
		}
	}
	if sigCount != 4 {
		t.Fatalf("got %d RRSIGs, want 4 (one per distinct RRset)", sigCount)
	}
	if len(signed) != len(rrs)+4 {
		t.Fatalf("got %d records, want %d (originals + 4 RRSIGs)", len(signed), len(rrs)+4)
	}
}

// TestSignZoneContentRRSIGsCarryTheCoveredRRsetsTTL proves a real,
// previously-shipped bug stays fixed: RFC 4034 §3 requires an RRSIG's own
// TTL to match the TTL of the RRset it covers, but miekg/dns's
// RRSIG.Sign only sets OrigTtl (the RDATA field carried inside the
// signed data) and deliberately leaves Hdr.Ttl -- the RRSIG's own wire
// TTL -- for the caller to set. Left unset, it silently defaults to
// zero. Found against a real validating resolver (Unbound): a zero-TTL
// RRSIG gets dropped from its cache immediately upon receipt, which
// corrupts its own multi-step recursive validation state and produces
// an opaque SERVFAIL ("Cannot retrieve DS for signature") for an
// otherwise completely valid, correctly signed answer.
func TestSignZoneContentRRSIGsCarryTheCoveredRRsetsTTL(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	a := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a.Hdr.Ttl = 300

	now := time.Now()
	signed, err := SignZoneContent([]dns.RR{a}, dnskeyRR, priv, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}
	for _, rr := range signed {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}
		if sig.Hdr.Ttl != a.Hdr.Ttl {
			t.Fatalf("RRSIG Hdr.Ttl = %d, want %d (the covered A record's TTL, per RFC 4034 §3)", sig.Hdr.Ttl, a.Hdr.Ttl)
		}
		if sig.OrigTtl != a.Hdr.Ttl {
			t.Fatalf("RRSIG OrigTtl = %d, want %d", sig.OrigTtl, a.Hdr.Ttl)
		}
	}
}

func TestSignZoneContentRRSIGsActuallyVerify(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	a := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	now := time.Now()

	signed, err := SignZoneContent([]dns.RR{dnskeyRR, a}, dnskeyRR, priv, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}

	for _, rr := range signed {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}
		var rrset []dns.RR
		for _, candidate := range signed {
			if candidate.Header().Rrtype == sig.TypeCovered {
				rrset = append(rrset, candidate)
			}
		}
		if err := sig.Verify(dnskeyRR, rrset); err != nil {
			t.Fatalf("RRSIG covering %s does not verify: %v", dns.TypeToString[sig.TypeCovered], err)
		}
	}
}

func TestVerifySignedRRsetsAcceptsGenuinelySignedContent(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	a := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a.Hdr.Class = dns.ClassINET
	dnskeyRR.Hdr.Class = dns.ClassINET
	now := time.Now()

	signed, err := SignZoneContent([]dns.RR{dnskeyRR, a}, dnskeyRR, priv, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}
	for _, rr := range signed {
		if sig, ok := rr.(*dns.RRSIG); ok {
			sig.Hdr.Class = dns.ClassINET // Sign() sets Class from rrset[0], already INET here, but be explicit
		}
	}

	if err := VerifySignedRRsets(dnskeyRR, signed, dns.ClassINET, now); err != nil {
		t.Fatalf("expected genuinely signed content to verify, got: %v", err)
	}
}

func TestVerifySignedRRsetsRejectsUnsignedRRset(t *testing.T) {
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	a := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a.Hdr.Class = dns.ClassINET

	if err := VerifySignedRRsets(dnskeyRR, []dns.RR{a}, dns.ClassINET, time.Now()); err == nil {
		t.Fatalf("expected an unsigned RRset to be rejected")
	}
}

func TestVerifySignedRRsetsRejectsWrongKeySignature(t *testing.T) {
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	otherKey, otherPriv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating other key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	otherDNSKEY := dnskeyRRFor(otherKey)
	a := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	a.Hdr.Class = dns.ClassINET
	otherDNSKEY.Hdr.Class = dns.ClassINET

	now := time.Now()
	// Signed by otherKey, but verified against the pinned key -- must fail.
	signed, err := SignZoneContent([]dns.RR{otherDNSKEY, a}, otherDNSKEY, otherPriv, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}

	if err := VerifySignedRRsets(dnskeyRR, signed, dns.ClassINET, now); err == nil {
		t.Fatalf("expected content signed by a different key to be rejected against the pinned key")
	}
}

func TestVerifySignedRRsetsRejectsExpiredSignature(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	dnskeyRR.Hdr.Class = dns.ClassINET
	now := time.Now()

	signed, err := SignZoneContent([]dns.RR{dnskeyRR}, dnskeyRR, priv, now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}

	if err := VerifySignedRRsets(dnskeyRR, signed, dns.ClassINET, now); err == nil {
		t.Fatalf("expected an already-expired RRSIG to be rejected")
	}
}

func TestVerifySignedRRsetsIgnoresDeleteShapedOps(t *testing.T) {
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	del := &dns.A{Hdr: dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeA, Class: dns.ClassNONE}, A: net.IPv4(203, 0, 113, 10)}

	if err := VerifySignedRRsets(dnskeyRR, []dns.RR{del}, dns.ClassINET, time.Now()); err != nil {
		t.Fatalf("expected a delete-shaped op to need no signature, got: %v", err)
	}
}
