package sazu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const testZoneFile = `$ORIGIN example.org.
@   3600 IN SOA ns1.example.org. hostmaster.example.org. 2024010100 3600 900 604800 3600
@   3600 IN NS  ns1.example.org.
www 300  IN A   203.0.113.10
mx  300  IN A   203.0.113.11
@   3600 IN MX  10 mx.example.org.
`

func writeTestZone(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "example.org.zone")
	if err := os.WriteFile(path, []byte(testZoneFile), 0o600); err != nil {
		t.Fatalf("writing test zone file: %v", err)
	}
	return path
}

func TestLoadZoneFileSeparatesSOAFromOtherRecords(t *testing.T) {
	path := writeTestZone(t)
	soa, rrs, err := LoadZoneFile(path, "example.org.")
	if err != nil {
		t.Fatalf("LoadZoneFile: %v", err)
	}
	if soa.Serial != 2024010100 {
		t.Fatalf("got serial %d, want 2024010100", soa.Serial)
	}
	if len(rrs) != 4 { // NS, www A, mx A, MX -- SOA excluded
		t.Fatalf("got %d non-SOA records, want 4", len(rrs))
	}
	for _, rr := range rrs {
		if _, isSOA := rr.(*dns.SOA); isSOA {
			t.Fatalf("SOA record leaked into the non-SOA record list")
		}
	}
}

func TestLoadZoneFileRejectsMissingSOA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-soa.zone")
	if err := os.WriteFile(path, []byte("$ORIGIN example.org.\nwww 300 IN A 203.0.113.10\n"), 0o600); err != nil {
		t.Fatalf("writing zone file: %v", err)
	}
	if _, _, err := LoadZoneFile(path, "example.org."); err == nil {
		t.Fatalf("expected an error for a zone file with no SOA record")
	}
}

func TestBuildFullZonePushShapesPrerequisiteAndUpdateSections(t *testing.T) {
	path := writeTestZone(t)
	soa, rrs, err := LoadZoneFile(path, "example.org.")
	if err != nil {
		t.Fatalf("LoadZoneFile: %v", err)
	}
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	previousSOA := *soa
	previousSOA.Serial-- // simulate "what the client last saw published"
	m, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, &previousSOA)
	if err != nil {
		t.Fatalf("BuildFullZonePush: %v", err)
	}

	if m.Opcode != dns.OpcodeUpdate {
		t.Fatalf("got opcode %d, want Update", m.Opcode)
	}
	if len(m.Question) != 1 || m.Question[0].Name != "example.org." {
		t.Fatalf("unexpected zone section: %+v", m.Question)
	}

	// Prerequisite section (Answer, per RFC 2136's field reuse): the
	// *previous* SOA, guarding against a stale push -- not the new one
	// being pushed.
	if len(m.Answer) != 1 {
		t.Fatalf("expected exactly one prerequisite, got %d", len(m.Answer))
	}
	gotSOA, ok := m.Answer[0].(*dns.SOA)
	if !ok || gotSOA.Serial != previousSOA.Serial {
		t.Fatalf("expected the SOA-serial staleness prerequisite against the previous serial, got %+v", m.Answer[0])
	}

	// Update section (Ns): the DNSKEY, the new SOA, every non-SOA record
	// from the zone file, and one RRSIG per distinct RRset among those --
	// nothing dropped or duplicated, and everything actually signed.
	var nonSigs []dns.RR
	var sigs []*dns.RRSIG
	for _, rr := range m.Ns {
		if sig, ok := rr.(*dns.RRSIG); ok {
			sigs = append(sigs, sig)
		} else {
			nonSigs = append(nonSigs, rr)
		}
	}
	if len(nonSigs) != len(rrs)+2 {
		t.Fatalf("got %d non-RRSIG update ops, want %d (%d zone records + DNSKEY + SOA)", len(nonSigs), len(rrs)+2, len(rrs))
	}
	wantSigs := len(groupRRsets(nonSigs))
	if len(sigs) != wantSigs {
		t.Fatalf("got %d RRSIGs, want %d (one per distinct RRset)", len(sigs), wantSigs)
	}
	dnskeyRR, ok := nonSigs[0].(*dns.DNSKEY)
	if !ok || dnskeyRR.PublicKey != key.PublicKey {
		t.Fatalf("expected the candidate DNSKEY to be the first update op, got %+v", nonSigs[0])
	}
	newSOA, ok := nonSigs[1].(*dns.SOA)
	if !ok || newSOA.Serial != soa.Serial {
		t.Fatalf("expected the new SOA to be pushed as content (second update op), got %+v", nonSigs[1])
	}
	for _, sig := range sigs {
		var rrset []dns.RR
		for _, rr := range nonSigs {
			if rr.Header().Rrtype == sig.TypeCovered && strings.EqualFold(rr.Header().Name, sig.Hdr.Name) {
				rrset = append(rrset, rr)
			}
		}
		if err := sig.Verify(dnskeyRR, rrset); err != nil {
			t.Fatalf("RRSIG covering %s/%s does not verify: %v", sig.Hdr.Name, dns.TypeToString[sig.TypeCovered], err)
		}
	}
}

func TestBuildFullZonePushFirstContactHasNoPrerequisite(t *testing.T) {
	path := writeTestZone(t)
	soa, rrs, err := LoadZoneFile(path, "example.org.")
	if err != nil {
		t.Fatalf("LoadZoneFile: %v", err)
	}
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	m, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("BuildFullZonePush: %v", err)
	}

	if len(m.Answer) != 0 {
		t.Fatalf("expected no prerequisites on a first-contact push, got %d", len(m.Answer))
	}
}

// TestBuildFullZonePushAcceptsHandBuiltRecords proves BuildFullZonePush
// itself has no dependency on the records having come from a real zone
// file -- any caller-constructed SOA/RRs work the same way. sazuctl's
// push-zone command requires a zone file (LoadZoneFile) regardless; this
// only exercises the underlying library function.
func TestBuildFullZonePushAcceptsHandBuiltRecords(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	soa := synthesizeSOA("example.org.")
	ns := &dns.NS{Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: soa.Ns}

	m, err := BuildFullZonePush("example.org.", soa, []dns.RR{ns}, key, priv, nil)
	if err != nil {
		t.Fatalf("BuildFullZonePush: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if err := VerifySIG0(wire, key); err != nil {
		t.Fatalf("a hand-built push should self-verify: %v", err)
	}
}

func TestBuildFullZonePushSignsAndVerifies(t *testing.T) {
	path := writeTestZone(t)
	soa, rrs, err := LoadZoneFile(path, "example.org.")
	if err != nil {
		t.Fatalf("LoadZoneFile: %v", err)
	}
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	m, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("BuildFullZonePush: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing full-zone push: %v", err)
	}
	if err := VerifySIG0(wire, key); err != nil {
		t.Fatalf("full-zone push should self-verify: %v", err)
	}
}
