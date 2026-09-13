package sazu

import (
	"net"
	"testing"

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
