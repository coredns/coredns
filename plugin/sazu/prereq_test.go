package sazu

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

// roundTrip packs and unpacks m, so its RRs carry wire-accurate Rdlength
// -- exactly the property EvaluatePrerequisites/ApplyUpdateOps rely on to
// distinguish RFC 2136's placeholder forms from real content, and which a
// freshly-constructed-in-Go RR would not otherwise have.
func roundTrip(t *testing.T, m *dns.Msg) *dns.Msg {
	t.Helper()
	buf, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	out := new(dns.Msg)
	if err := out.Unpack(buf); err != nil {
		t.Fatalf("unpacking: %v", err)
	}
	return out
}

func baseUpdateMsg(zone string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	return m
}

func TestEvaluatePrerequisitesNameIsInUse(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))

	m := baseUpdateMsg("example.org.")
	m.NameUsed([]dns.RR{testA("www.example.org.", net.IPv4(1, 2, 3, 4))})
	m = roundTrip(t, m)

	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err == nil || rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN for a name that isn't in use, got rcode=%d err=%v", rcode, err)
	}

	z.Insert(testA("www.example.org.", net.IPv4(1, 2, 3, 4)))
	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err != nil {
		t.Fatalf("expected success once the name exists, got rcode=%d err=%v", rcode, err)
	}
}

func TestEvaluatePrerequisitesNameNotInUse(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))

	m := baseUpdateMsg("example.org.")
	m.NameNotUsed([]dns.RR{testA("www.example.org.", net.IPv4(1, 2, 3, 4))})
	m = roundTrip(t, m)

	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err != nil {
		t.Fatalf("expected success for an unused name, got rcode=%d err=%v", rcode, err)
	}

	z.Insert(testA("www.example.org.", net.IPv4(1, 2, 3, 4)))
	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err == nil || rcode != dns.RcodeYXDomain {
		t.Fatalf("expected YXDOMAIN once the name exists, got rcode=%d err=%v", rcode, err)
	}
}

func TestEvaluatePrerequisitesRRsetExistsValueIndependent(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))

	m := baseUpdateMsg("example.org.")
	m.RRsetUsed([]dns.RR{testA("www.example.org.", nil)})
	m = roundTrip(t, m)

	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err == nil || rcode != dns.RcodeNXRrset {
		t.Fatalf("expected NXRRSET for a missing rrset, got rcode=%d err=%v", rcode, err)
	}

	z.Insert(testA("www.example.org.", net.IPv4(1, 2, 3, 4)))
	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err != nil {
		t.Fatalf("expected success once the rrset exists, got rcode=%d err=%v", rcode, err)
	}
}

func TestEvaluatePrerequisitesRRsetDoesNotExist(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(1))

	m := baseUpdateMsg("example.org.")
	m.RRsetNotUsed([]dns.RR{testA("www.example.org.", nil)})
	m = roundTrip(t, m)

	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err != nil {
		t.Fatalf("expected success for a genuinely missing rrset, got rcode=%d err=%v", rcode, err)
	}

	z.Insert(testA("www.example.org.", net.IPv4(1, 2, 3, 4)))
	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err == nil || rcode != dns.RcodeYXRrset {
		t.Fatalf("expected YXRRSET once the rrset exists, got rcode=%d err=%v", rcode, err)
	}
}

func TestEvaluatePrerequisitesRRsetExistsValueDependentSOAStalenessGuard(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testSOA(5))

	m := baseUpdateMsg("example.org.")
	m.Used([]dns.RR{testSOA(5)}) // matches what's actually published
	m = roundTrip(t, m)
	if rcode, err := EvaluatePrerequisites(z, m.Answer, dns.ClassINET); err != nil {
		t.Fatalf("expected success when the prerequisite SOA matches, got rcode=%d err=%v", rcode, err)
	}

	stale := baseUpdateMsg("example.org.")
	stale.Used([]dns.RR{testSOA(3)}) // a push built from an older snapshot
	stale = roundTrip(t, stale)
	if rcode, err := EvaluatePrerequisites(z, stale.Answer, dns.ClassINET); err == nil || rcode != dns.RcodeNXRrset {
		t.Fatalf("expected a stale-serial push to be rejected with NXRRSET, got rcode=%d err=%v", rcode, err)
	}
}

func TestApplyUpdateOpsAddToRRset(t *testing.T) {
	z := NewZoneData("example.org.")
	m := baseUpdateMsg("example.org.")
	m.Insert([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	m = roundTrip(t, m)

	if err := ApplyUpdateOps(z, m.Ns, dns.ClassINET); err != nil {
		t.Fatalf("ApplyUpdateOps: %v", err)
	}
	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 1 {
		t.Fatalf("expected the A record to have been added, got %d", len(got))
	}
}

func TestApplyUpdateOpsDeleteRRset(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))

	m := baseUpdateMsg("example.org.")
	m.RemoveRRset([]dns.RR{testA("www.example.org.", nil)})
	m = roundTrip(t, m)

	if err := ApplyUpdateOps(z, m.Ns, dns.ClassINET); err != nil {
		t.Fatalf("ApplyUpdateOps: %v", err)
	}
	if got := z.Lookup("www.example.org.", dns.TypeA); len(got) != 0 {
		t.Fatalf("expected the rrset to have been deleted, got %d", len(got))
	}
}

func TestApplyUpdateOpsDeleteAllRRsetsFromName(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(&dns.MX{Hdr: dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 300}, Preference: 10, Mx: "mx.example.org."})

	m := baseUpdateMsg("example.org.")
	m.RemoveName([]dns.RR{testA("www.example.org.", nil)})
	m = roundTrip(t, m)

	if err := ApplyUpdateOps(z, m.Ns, dns.ClassINET); err != nil {
		t.Fatalf("ApplyUpdateOps: %v", err)
	}
	if z.NameExists("www.example.org.") {
		t.Fatalf("expected the name to no longer exist")
	}
}

func TestApplyUpdateOpsDeleteOneRR(t *testing.T) {
	z := NewZoneData("example.org.")
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 10)))
	z.Insert(testA("www.example.org.", net.IPv4(203, 0, 113, 11)))

	m := baseUpdateMsg("example.org.")
	m.Remove([]dns.RR{testA("www.example.org.", net.IPv4(203, 0, 113, 10))})
	m = roundTrip(t, m)

	if err := ApplyUpdateOps(z, m.Ns, dns.ClassINET); err != nil {
		t.Fatalf("ApplyUpdateOps: %v", err)
	}
	got := z.Lookup("www.example.org.", dns.TypeA)
	if len(got) != 1 || !got[0].(*dns.A).A.Equal(net.IPv4(203, 0, 113, 11)) {
		t.Fatalf("expected only .11 to remain, got %+v", got)
	}
}
