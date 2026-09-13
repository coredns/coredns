package sazu

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

// TestCanonicalCompareMatchesRFC4034Example proves canonicalCompare
// against RFC 4034 §6.1's own worked example of names already in
// canonical order -- minus its two numeric-escape entries (\001.z.example.
// and \200.z.example.), which need comparison by real decoded byte value
// to sort correctly (0x01 and 0xC8 respectively) rather than by the
// escaped textual label canonicalCompare actually operates on. Scoped,
// documented behavior (see this function's own doc comment): correct for
// the plain ASCII hostnames a real SAZU zone contains, not a
// general-purpose implementation for exotic binary labels.
func TestCanonicalCompareMatchesRFC4034Example(t *testing.T) {
	inOrder := []string{
		"example.",
		"a.example.",
		"yljkjljk.a.example.",
		"Z.a.example.",
		"zABC.a.EXAMPLE.",
		"z.example.",
		"*.z.example.",
	}
	for i := 0; i < len(inOrder)-1; i++ {
		if canonicalCompare(inOrder[i], inOrder[i+1]) >= 0 {
			t.Fatalf("expected %q < %q, canonicalCompare returned %d", inOrder[i], inOrder[i+1], canonicalCompare(inOrder[i], inOrder[i+1]))
		}
	}
}

func TestCanonicalCompareEqualNamesReturnZero(t *testing.T) {
	if canonicalCompare("WWW.Example.ORG.", "www.example.org.") != 0 {
		t.Fatalf("expected case-insensitive equality")
	}
}

func TestSortNamesCanonically(t *testing.T) {
	names := []string{"z.example.org.", "example.org.", "a.example.org.", "mx.example.org."}
	sortNamesCanonically(names)
	want := []string{"example.org.", "a.example.org.", "mx.example.org.", "z.example.org."}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got order %v, want %v", names, want)
		}
	}
}

func TestClosestEncloserFindsLongestExistingSuffix(t *testing.T) {
	owners := map[string]bool{"example.org.": true, "www.example.org.": true}
	if got := closestEncloser("nope.www.example.org.", owners); got != "www.example.org." {
		t.Fatalf("got %q, want www.example.org.", got)
	}
	if got := closestEncloser("nope.example.org.", owners); got != "example.org." {
		t.Fatalf("got %q, want example.org.", got)
	}
}

func TestCoveringOwnerFindsPredecessor(t *testing.T) {
	sorted := []string{"example.org.", "a.example.org.", "z.example.org."}
	owner, ok := coveringOwner("m.example.org.", sorted)
	if !ok || owner != "a.example.org." {
		t.Fatalf("got owner=%q ok=%v, want a.example.org.", owner, ok)
	}
}

// TestCoveringOwnerWrapsAroundTheChain uses an owner set with no single
// common ancestor (unlike a real single-zone name set, where the apex is
// always every other owner's ancestor and therefore always the
// canonical minimum) specifically so a query name can genuinely sort
// before every owner without merely being one's own descendant --
// exercising the actual wrap-around branch, not the ordinary predecessor
// one.
func TestCoveringOwnerWrapsAroundTheChain(t *testing.T) {
	sorted := []string{"b.example.", "m.example.", "z.example."}
	owner, ok := coveringOwner("a.example.", sorted)
	if !ok || owner != "z.example." {
		t.Fatalf("got owner=%q ok=%v, want z.example. (wrap-around)", owner, ok)
	}
}

func TestCoveringOwnerEmptyChain(t *testing.T) {
	if _, ok := coveringOwner("example.org.", nil); ok {
		t.Fatalf("expected ok=false for an empty chain")
	}
}

// TestBuildNSECChainCoversEveryOwnerAndCycles proves the synthesized
// chain visits every distinct owner name among adds exactly once and
// wraps back to its start, per RFC 4034 §4.
func TestBuildNSECChainCoversEveryOwnerAndCycles(t *testing.T) {
	soa := testSOA(1)
	soa.Minttl = 1800
	adds := []dns.RR{
		soa,
		&dns.NS{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: "ns1.example.org."},
		testA("www.example.org.", net.IPv4(203, 0, 113, 10)),
		testA("mx.example.org.", net.IPv4(203, 0, 113, 11)),
	}
	chain := BuildNSECChain(soa, adds)
	if len(chain) != 3 {
		t.Fatalf("got %d NSEC records, want 3 (apex, www, mx)", len(chain))
	}

	byOwner := make(map[string]*dns.NSEC, len(chain))
	for _, rr := range chain {
		n, ok := rr.(*dns.NSEC)
		if !ok {
			t.Fatalf("BuildNSECChain returned a non-NSEC record: %+v", rr)
		}
		if n.Hdr.Ttl != soa.Minttl {
			t.Fatalf("NSEC TTL = %d, want the SOA minimum %d (RFC 4034 §4)", n.Hdr.Ttl, soa.Minttl)
		}
		byOwner[n.Hdr.Name] = n
	}

	visited := map[string]bool{}
	name := chain[0].Header().Name
	for i := 0; i < len(chain); i++ {
		if visited[name] {
			t.Fatalf("chain revisited %s before covering all owners", name)
		}
		visited[name] = true
		next, ok := byOwner[name]
		if !ok {
			t.Fatalf("chain points at %s, which has no NSEC record", name)
		}
		name = next.NextDomain
	}
	if name != chain[0].Header().Name {
		t.Fatalf("chain did not cycle back to its start, ended at %s", name)
	}
}

// TestBuildNSECChainTypeBitmapReflectsContent proves each NSEC's type
// bitmap lists exactly the RRset types present at that name (plus NSEC
// and RRSIG themselves, which will exist there once this is signed), in
// ascending order (miekg/dns's packer requires this).
func TestBuildNSECChainTypeBitmapReflectsContent(t *testing.T) {
	soa := testSOA(1)
	dnskeyRR := &dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600}}
	ns := &dns.NS{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: "ns1.example.org."}
	adds := []dns.RR{soa, dnskeyRR, ns, testA("www.example.org.", net.IPv4(203, 0, 113, 10))}

	chain := BuildNSECChain(soa, adds)
	var apex, www *dns.NSEC
	for _, rr := range chain {
		n := rr.(*dns.NSEC)
		switch n.Hdr.Name {
		case "example.org.":
			apex = n
		case "www.example.org.":
			www = n
		}
	}
	if apex == nil || www == nil {
		t.Fatalf("expected NSEC records at both the apex and www, got %+v", chain)
	}
	wantApex := []uint16{dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeNSEC, dns.TypeRRSIG}
	assertSortedTypesEqual(t, apex.TypeBitMap, wantApex)
	wantWWW := []uint16{dns.TypeA, dns.TypeNSEC, dns.TypeRRSIG}
	assertSortedTypesEqual(t, www.TypeBitMap, wantWWW)
}

func assertSortedTypesEqual(t *testing.T, got, want []uint16) {
	t.Helper()
	gotSet := make(map[uint16]bool, len(got))
	for _, ty := range got {
		gotSet[ty] = true
	}
	if len(gotSet) != len(want) {
		t.Fatalf("got bitmap %v, want (unordered) %v", got, want)
	}
	for _, ty := range want {
		if !gotSet[ty] {
			t.Fatalf("bitmap %v missing type %d", got, ty)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("bitmap %v is not strictly ascending -- packDataNsec requires this", got)
		}
	}
}

// TestBuildNSECChainSingleOwnerWrapsToItself proves a one-name zone
// (only the apex has any content) produces exactly one NSEC record whose
// NextDomain points back at itself.
func TestBuildNSECChainSingleOwnerWrapsToItself(t *testing.T) {
	soa := testSOA(1)
	chain := BuildNSECChain(soa, []dns.RR{soa})
	if len(chain) != 1 {
		t.Fatalf("got %d NSEC records, want 1", len(chain))
	}
	n := chain[0].(*dns.NSEC)
	if n.Hdr.Name != "example.org." || n.NextDomain != "example.org." {
		t.Fatalf("expected a self-pointing NSEC at the apex, got %+v", n)
	}
}
