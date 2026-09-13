package sazu

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func testSOA(serial uint32) *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      "ns1.example.org.",
		Mbox:    "hostmaster.example.org.",
		Serial:  serial,
		Refresh: 3600, Retry: 900, Expire: 604800, Minttl: 3600,
	}
}

func testA(name string, ip net.IP) *dns.A {
	return &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: ip}
}

// synthesizeSOA builds a throwaway SOA for zone, for tests that need one
// for an arbitrary zone name rather than the fixed "example.org." testSOA
// uses. Onboarding for real always goes through a real zone file
// (LoadZoneFile) -- this has no production equivalent.
func synthesizeSOA(zone string) *dns.SOA {
	zone = dns.Fqdn(zone)
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      "ns1." + zone,
		Mbox:    "hostmaster." + zone,
		Serial:  uint32(time.Now().Unix()),
		Refresh: 3600, Retry: 900, Expire: 604800, Minttl: 3600,
	}
}

func TestZoneDataInsertAndLookupSOA(t *testing.T) {
	z := NewZoneData("example.org.")
	if z.SOA() != nil {
		t.Fatalf("expected no SOA before anything is inserted")
	}
	z.Insert(testSOA(1))
	if got := z.SOA(); got == nil || got.Serial != 1 {
		t.Fatalf("expected SOA serial 1, got %+v", got)
	}
	// A second SOA replaces, rather than appends to, the tracked one.
	z.Insert(testSOA(2))
	if got := z.SOA(); got == nil || got.Serial != 2 {
		t.Fatalf("expected SOA to be replaced with serial 2, got %+v", got)
	}
}

func TestZoneDataInsertAndLookupA(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 {
		t.Fatalf("expected 1 A record, got %d", len(got))
	}
	a, ok := got[0].(*dns.A)
	if !ok || !a.A.Equal(net.IPv4(203, 0, 113, 10)) {
		t.Fatalf("unexpected record: %+v", got[0])
	}
}

// TestZoneDataInsertOfIdenticalRDataDoesNotDuplicate proves RFC 2136
// §3.4.2.2's "duplicate RDATA replaces" rule: inserting an RR whose
// content (ignoring TTL) already exists in the RRset must not produce a
// second, redundant copy -- e.g. re-onboarding or re-pushing a zone
// whose non-signature content hasn't actually changed.
func TestZoneDataInsertOfIdenticalRDataDoesNotDuplicate(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10))) // identical content, pushed again

	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 {
		t.Fatalf("expected the duplicate insert to be a no-op (1 record), got %d: %+v", len(got), got)
	}
}

// TestZoneDataInsertOfIdenticalRDataRefreshesTTL proves the "replaces"
// half of the same RFC 2136 rule: the TTL on the (sole) surviving record
// tracks the most recently inserted value, rather than the insert being
// silently dropped outright.
func TestZoneDataInsertOfIdenticalRDataRefreshesTTL(t *testing.T) {
	z := NewZoneData("example.org.")
	first := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	first.Hdr.Ttl = 300
	z.Insert(first)

	second := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	second.Hdr.Ttl = 600
	z.Insert(second)

	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 || got[0].Header().Ttl != 600 {
		t.Fatalf("expected 1 record with the refreshed TTL 600, got %+v", got)
	}
}

// TestZoneDataInsertOfDifferentContentDoesNotReplace proves the dedup fix
// only collapses genuinely identical content -- two A records at the same
// name with different addresses are a real multi-value RRset, not a
// duplicate.
func TestZoneDataInsertOfDifferentContentDoesNotReplace(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 11)))

	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct A records, got %d: %+v", len(got), got)
	}
}

// TestZoneDataInsertRRSIGReplacesSameSignersPreviousSignature proves the
// fix for real, previously-undiscovered unbounded growth: re-signing an
// RRset (e.g. ahead of the previous signature's expiry) produces a fresh
// RRSIG whose RDATA -- the signature bytes and validity window -- always
// differs from the last one, even when the covered content hasn't
// changed at all. RFC 2136's literal "identical RDATA replaces" rule
// can't catch that, so without this, every re-sign of a long-lived zone
// would accumulate one more RRSIG forever. SAZU's single-key design means
// at most one active signature per (RRset, signer) is ever wanted, so a
// fresh RRSIG from the same signer replaces its own previous one.
func TestZoneDataInsertRRSIGReplacesSameSignersPreviousSignature(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	older := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 300},
		TypeCovered: dns.TypeA, Algorithm: 15, KeyTag: 1234, SignerName: "example.org.",
		Inception: 1000, Expiration: 2000, Signature: "old-signature-bytes",
	}
	z.Insert(older)

	newer := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 300},
		TypeCovered: dns.TypeA, Algorithm: 15, KeyTag: 1234, SignerName: "example.org.",
		Inception: 5000, Expiration: 6000, Signature: "new-signature-bytes",
	}
	z.Insert(newer)

	got := z.LookupRRSIG("www.example.org.", dns.TypeA)
	if len(got) != 1 {
		t.Fatalf("expected the re-sign to replace, not accumulate, RRSIGs; got %d: %+v", len(got), got)
	}
	sig, ok := got[0].(*dns.RRSIG)
	if !ok || sig.Signature != "new-signature-bytes" {
		t.Fatalf("expected only the newer signature to survive, got %+v", got[0])
	}
}

// TestZoneDataInsertRRSIGFromDifferentSignerCoexists proves the replace
// logic is scoped to "same signer, same covered type, same key" -- a
// second key legitimately signing the same RRset (e.g. mid key rollover)
// must not evict the first signer's still-valid signature.
func TestZoneDataInsertRRSIGFromDifferentSignerCoexists(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	fromKeyA := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 300},
		TypeCovered: dns.TypeA, Algorithm: 15, KeyTag: 1111, SignerName: "example.org.",
		Inception: 1000, Expiration: 2000, Signature: "key-a-signature",
	}
	fromKeyB := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 300},
		TypeCovered: dns.TypeA, Algorithm: 15, KeyTag: 2222, SignerName: "example.org.",
		Inception: 1000, Expiration: 2000, Signature: "key-b-signature",
	}
	z.Insert(fromKeyA)
	z.Insert(fromKeyB)

	got := z.LookupRRSIG("www.example.org.", dns.TypeA)
	if len(got) != 2 {
		t.Fatalf("expected both signers' RRSIGs to coexist, got %d: %+v", len(got), got)
	}
}

func TestZoneDataLookupReturnsIndependentCopies(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	got := z.Lookup("www.example.org.", dns.TypeA)
	got[0].(*dns.A).A = net.IPv4(198, 51, 100, 1) // mutate the caller's copy

	got2 := z.Lookup("www.example.org.", dns.TypeA)
	if !got2[0].(*dns.A).A.Equal(net.IPv4(203, 0, 113, 10)) {
		t.Fatalf("mutating a looked-up record leaked into the store: %+v", got2[0])
	}
}

func TestZoneDataDeleteRRset(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 11)))

	z.DeleteRRset("www.example.org.", dns.TypeA)
	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 0 {
		t.Fatalf("expected the RRset to be gone, got %d records", len(got))
	}
}

func TestZoneDataDeleteRRsetCannotRemoveApexSOA(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))
	z.DeleteRRset("example.org.", dns.TypeSOA)
	if z.SOA() == nil {
		t.Fatalf("expected the apex SOA to survive a delete-RRset op")
	}
}

func TestZoneDataDeleteName(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(&dns.MX{Hdr: dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 300}, Preference: 10, Mx: "mx.example.org."})

	z.DeleteName("www.example.org.")
	if z.NameExists("www.example.org.") {
		t.Fatalf("expected the name to no longer exist after DeleteName")
	}
}

func TestZoneDataDeleteNameCannotRemoveApex(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))
	z.DeleteName("example.org.")
	if z.SOA() == nil {
		t.Fatalf("expected DeleteName on the apex to be a no-op for the SOA")
	}
}

func TestZoneDataDeleteRRIgnoresTTL(t *testing.T) {
	z := NewZoneData("example.org.")
	rr := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	z.Insert(rr)

	del := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	del.Hdr.Ttl = 9999 // different TTL, same content -- RFC 2136 deletes ignore TTL
	z.DeleteRR(del)

	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 0 {
		t.Fatalf("expected the record to be deleted despite the TTL mismatch, got %d", len(got))
	}
}

func TestZoneDataDeleteRRLeavesOtherRecordsInRRset(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 11)))

	z.DeleteRR(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 || !got[0].(*dns.A).A.Equal(net.IPv4(203, 0, 113, 11)) {
		t.Fatalf("expected only .11 to remain, got %+v", got)
	}
}

func TestStoreGetOrCreateReturnsSameZoneOnRepeatedCalls(t *testing.T) {
	s := NewStore()
	z1 := s.GetOrCreate("example.org.")
	z1.Insert(testSOA(1))

	z2 := s.GetOrCreate("example.org.")
	if z2.SOA() == nil || z2.SOA().Serial != 1 {
		t.Fatalf("expected GetOrCreate to return the same zone instance, got a fresh one")
	}
}

func TestStoreGetReportsAbsence(t *testing.T) {
	s := NewStore()
	if _, ok := s.Get("example.org."); ok {
		t.Fatalf("expected Get to report absence for a zone never created")
	}
}

func testNSEC(name, next string, types ...uint16) *dns.NSEC {
	return &dns.NSEC{
		Hdr:        dns.RR_Header{Name: name, Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: 3600},
		NextDomain: next,
		TypeBitMap: types,
	}
}

func testRRSIGCoveringNSEC(name string, expiration uint32) *dns.RRSIG {
	return &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: name, Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 3600},
		TypeCovered: dns.TypeNSEC, Algorithm: 15, KeyTag: 1, SignerName: "example.org.",
		Expiration: expiration,
	}
}

// TestZoneDataInsertNSECReplacesRatherThanAccumulates proves NSEC is
// treated as a singleton per name -- unlike an ordinary RRset, a second,
// differently-valued NSEC at the same name (e.g. after the zone's name
// set changed) replaces the first outright, since RFC 2136's own
// "identical RDATA replaces" rule can't catch two NSEC records whose
// NextDomain genuinely differs.
func TestZoneDataInsertNSECReplacesRatherThanAccumulates(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testNSEC("example.org.", "a.example.org.", dns.TypeNSEC))
	z.Insert(testNSEC("example.org.", "b.example.org.", dns.TypeNSEC))

	got := z.Lookup("example.org.", dns.TypeNSEC)
	if len(got) != 1 {
		t.Fatalf("expected exactly one NSEC to survive, got %d: %+v", len(got), got)
	}
	if got[0].(*dns.NSEC).NextDomain != "b.example.org." {
		t.Fatalf("expected the newer NSEC to have replaced the older, got %+v", got[0])
	}
}

func TestZoneDataPurgeNSECRemovesRecordsAndTheirRRSIGs(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testNSEC("example.org.", "www.example.org.", dns.TypeSOA, dns.TypeNSEC))
	z.Insert(testRRSIGCoveringNSEC("example.org.", 2000))
	z.Insert(testNSEC("www.example.org.", "example.org.", dns.TypeA, dns.TypeNSEC))
	z.Insert(testRRSIGCoveringNSEC("www.example.org.", 2000))

	z.PurgeNSEC()

	if got := z.Lookup("example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected the apex NSEC to be purged, got %+v", got)
	}
	if got := z.LookupRRSIG("example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected the apex NSEC's RRSIG to be purged, got %+v", got)
	}
	if got := z.Lookup("www.example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected www's NSEC to be purged, got %+v", got)
	}
	if got := z.LookupRRSIG("www.example.org.", dns.TypeNSEC); len(got) != 0 {
		t.Fatalf("expected www's NSEC RRSIG to be purged, got %+v", got)
	}
	// Purging NSEC must not touch unrelated content.
	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 1 {
		t.Fatalf("expected the A record to survive PurgeNSEC, got %+v", got)
	}
}

// TestZoneDataNegativeProofNODATAReturnsNSECAtQueriedName proves the
// NODATA case: the name exists, so the proof is simply whatever NSEC (+
// RRSIG) is stored at that exact name.
func TestZoneDataNegativeProofNODATAReturnsNSECAtQueriedName(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testNSEC("www.example.org.", "example.org.", dns.TypeA, dns.TypeNSEC))
	z.Insert(testRRSIGCoveringNSEC("www.example.org.", 2000))

	got := z.NegativeProof("www.example.org.", true)
	var sawNSEC, sawRRSIG bool
	for _, rr := range got {
		switch rr.(type) {
		case *dns.NSEC:
			sawNSEC = true
		case *dns.RRSIG:
			sawRRSIG = true
		}
	}
	if !sawNSEC || !sawRRSIG {
		t.Fatalf("expected both the NSEC and its RRSIG, got %+v", got)
	}
}

// TestZoneDataNegativeProofNXDOMAINCoversQnameAndWildcardSlot proves the
// NXDOMAIN case returns the NSEC covering the queried name itself, plus
// the NSEC covering the wildcard slot at its closest encloser (here, the
// same NSEC covers both, which is a legitimate and common outcome, not a
// bug -- NegativeProof must not report the same owner twice).
func TestZoneDataNegativeProofNXDOMAINCoversQnameAndWildcardSlot(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	// Apex covers everything up to www (including the wildcard slot
	// "*.example.org.", and any nonexistent name that sorts before www,
	// e.g. "aaa.example.org."); www wraps back around to the apex.
	z.Insert(testNSEC("example.org.", "www.example.org.", dns.TypeSOA, dns.TypeNS, dns.TypeNSEC))
	z.Insert(testRRSIGCoveringNSEC("example.org.", 2000))
	z.Insert(testNSEC("www.example.org.", "example.org.", dns.TypeA, dns.TypeNSEC))
	z.Insert(testRRSIGCoveringNSEC("www.example.org.", 2000))

	got := z.NegativeProof("aaa.example.org.", false)
	var owners []string
	for _, rr := range got {
		if n, ok := rr.(*dns.NSEC); ok {
			owners = append(owners, n.Hdr.Name)
		}
	}
	if len(owners) != 1 || owners[0] != "example.org." {
		t.Fatalf("expected exactly one NSEC, from the apex (which covers both the qname and the wildcard slot), got %+v", owners)
	}
}

func TestZoneDataNegativeProofReturnsNilWithNoChainPushed(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	if got := z.NegativeProof("nope.example.org.", false); len(got) != 0 {
		t.Fatalf("expected no proof when no NSEC chain was ever pushed, got %+v", got)
	}
	if got := z.NegativeProof("www.example.org.", true); len(got) != 0 {
		t.Fatalf("expected no proof when no NSEC chain was ever pushed, got %+v", got)
	}
}
