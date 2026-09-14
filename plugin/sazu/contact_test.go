package sazu

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func contactTXT(zone string, class uint16, txt ...string) *dns.TXT {
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: contactOwnerName(zone), Rrtype: dns.TypeTXT, Class: class, Ttl: 3600},
		Txt: txt,
	}
}

// asAddOps runs ops through a real Pack/Unpack round trip (via roundTrip's
// dns.Msg-level wrapper) so each carries wire-accurate Rdlength -- the
// property splitContactOps relies on, exactly like findCandidateKey and
// containsAPEXSOA, to tell a real Add op from an RFC 2136 placeholder
// delete-shaped one. A freshly-constructed-in-Go RR does not have this
// unless deliberately set, unlike every op this function sees for real
// (always wire-decoded from an actual UPDATE message).
func asAddOps(t *testing.T, ops []dns.RR) []dns.RR {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Insert(ops)
	return roundTrip(t, m).Ns
}

func TestSplitContactOpsExtractsAddOp(t *testing.T) {
	other := testA("www.example.org.", net.IPv4(203, 0, 113, 10))
	other.Hdr.Class = dns.ClassINET
	ops := asAddOps(t, []dns.RR{other, contactTXT("example.org.", dns.ClassINET, "mailto:ops@example.org")})

	rest, update, err := splitContactOps(ops, "example.org.")
	if err != nil {
		t.Fatalf("splitContactOps: %v", err)
	}
	if len(rest) != 1 || rest[0].Header().Name != other.Header().Name {
		t.Fatalf("expected the contact op stripped from rest, got %+v", rest)
	}
	if update == nil || len(update.Addresses) != 1 || update.Addresses[0] != "mailto:ops@example.org" {
		t.Fatalf("expected a contact update with one address, got %+v", update)
	}
}

func TestSplitContactOpsExtractsDeleteOp(t *testing.T) {
	del := &dns.TXT{Hdr: dns.RR_Header{Name: contactOwnerName("example.org."), Rrtype: dns.TypeTXT, Class: dns.ClassNONE}}
	rest, update, err := splitContactOps([]dns.RR{del}, "example.org.")
	if err != nil {
		t.Fatalf("splitContactOps: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("expected the delete op stripped from rest, got %+v", rest)
	}
	if update == nil || len(update.Addresses) != 0 {
		t.Fatalf("expected a clearing contact update, got %+v", update)
	}
}

func TestSplitContactOpsNoContactOpReturnsNilUpdate(t *testing.T) {
	a := testA("www.example.org.", nil)
	a.Hdr.Class = dns.ClassINET
	rest, update, err := splitContactOps([]dns.RR{a}, "example.org.")
	if err != nil {
		t.Fatalf("splitContactOps: %v", err)
	}
	if len(rest) != 1 {
		t.Fatalf("expected the non-contact op preserved, got %+v", rest)
	}
	if update != nil {
		t.Fatalf("expected no contact update when the push carries none, got %+v", update)
	}
}

func TestSplitContactOpsRejectsInvalidScheme(t *testing.T) {
	ops := asAddOps(t, []dns.RR{contactTXT("example.org.", dns.ClassINET, "not-a-uri")})
	if _, _, err := splitContactOps(ops, "example.org."); err == nil {
		t.Fatalf("expected an address without a recognized scheme to be rejected")
	}
}

func TestSplitContactOpsRejectsMoreThanOneDirective(t *testing.T) {
	ops := asAddOps(t, []dns.RR{
		contactTXT("example.org.", dns.ClassINET, "mailto:a@example.org"),
		contactTXT("example.org.", dns.ClassINET, "mailto:b@example.org"),
	})
	if _, _, err := splitContactOps(ops, "example.org."); err == nil {
		t.Fatalf("expected more than one contact directive in a single push to be rejected")
	}
}

func TestSplitContactOpsAcceptsMultipleAddressesInOneOp(t *testing.T) {
	ops := asAddOps(t, []dns.RR{contactTXT("example.org.", dns.ClassINET, "mailto:ops@example.org", "https://hooks.example.org/sazu")})
	_, update, err := splitContactOps(ops, "example.org.")
	if err != nil {
		t.Fatalf("splitContactOps: %v", err)
	}
	if len(update.Addresses) != 2 {
		t.Fatalf("expected both addresses kept, got %+v", update.Addresses)
	}
}

func TestValidateContactAddressesRejectsUnknownScheme(t *testing.T) {
	if _, err := validateContactAddresses([]string{"ftp://example.org"}); err == nil {
		t.Fatalf("expected an unrecognized scheme to be rejected")
	}
}

func TestValidateContactAddressesRejectsEmptyList(t *testing.T) {
	if _, err := validateContactAddresses(nil); err == nil {
		t.Fatalf("expected an empty address list to be rejected")
	}
	if _, err := validateContactAddresses([]string{"  "}); err == nil {
		t.Fatalf("expected an all-whitespace address list to be rejected")
	}
}

// TestSplitContactOpsDropsOrphanRRSIGOverTheContactRecord proves the
// defensive strip: a naively-built client might run the contact TXT
// through the same signing path as real zone content and send its RRSIG
// along too (BuildContactOp exists specifically so a well-behaved client
// doesn't), but that signature must never survive into zone content --
// it isn't zone content, and would otherwise be an orphan RRSIG for an
// RRset that's never actually served.
func TestSplitContactOpsDropsOrphanRRSIGOverTheContactRecord(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)
	txt := contactTXT("example.org.", dns.ClassINET, "mailto:ops@example.org")
	now := time.Now()
	signed, err := SignZoneContent([]dns.RR{txt}, dnskeyRR, priv, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignZoneContent: %v", err)
	}

	rest, update, err := splitContactOps(asAddOps(t, signed), "example.org.")
	if err != nil {
		t.Fatalf("splitContactOps: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("expected the orphan RRSIG over the contact record dropped, got %+v", rest)
	}
	if update == nil || len(update.Addresses) != 1 {
		t.Fatalf("expected the contact update itself still recognized, got %+v", update)
	}
}

func TestContactRegistrySetAndClear(t *testing.T) {
	r := NewContactRegistry()
	if _, ok := r.Get("example.org."); ok {
		t.Fatalf("expected no contact registered initially")
	}
	r.Set("example.org.", []string{"mailto:ops@example.org"})
	got, ok := r.Get("EXAMPLE.ORG.") // case-insensitive, like KeyRegistry
	if !ok || len(got) != 1 || got[0] != "mailto:ops@example.org" {
		t.Fatalf("expected the registered contact back, got %+v ok=%v", got, ok)
	}
	r.Set("example.org.", nil)
	if _, ok := r.Get("example.org."); ok {
		t.Fatalf("expected Set(nil) to clear the registered contact")
	}
}
