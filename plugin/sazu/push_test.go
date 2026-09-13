package sazu

import (
	"os"
	"path/filepath"
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
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	m := BuildFullZonePush("example.org.", soa, rrs, key)

	if m.Opcode != dns.OpcodeUpdate {
		t.Fatalf("got opcode %d, want Update", m.Opcode)
	}
	if len(m.Question) != 1 || m.Question[0].Name != "example.org." {
		t.Fatalf("unexpected zone section: %+v", m.Question)
	}

	// Prerequisite section (Answer, per RFC 2136's field reuse): exactly
	// the SOA, unmodified content, guarding against a stale push.
	if len(m.Answer) != 1 {
		t.Fatalf("expected exactly one prerequisite, got %d", len(m.Answer))
	}
	gotSOA, ok := m.Answer[0].(*dns.SOA)
	if !ok || gotSOA.Serial != soa.Serial {
		t.Fatalf("expected the SOA-serial staleness prerequisite, got %+v", m.Answer[0])
	}

	// Update section (Ns): the DNSKEY plus every non-SOA record from the
	// zone file, nothing dropped or duplicated.
	if len(m.Ns) != len(rrs)+1 {
		t.Fatalf("got %d update ops, want %d (%d zone records + 1 DNSKEY)", len(m.Ns), len(rrs)+1, len(rrs))
	}
	dnskeyRR, ok := m.Ns[0].(*dns.DNSKEY)
	if !ok || dnskeyRR.PublicKey != key.PublicKey {
		t.Fatalf("expected the candidate DNSKEY to be the first update op, got %+v", m.Ns[0])
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

	m := BuildFullZonePush("example.org.", soa, rrs, key)
	now := time.Now()
	wire, err := SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing full-zone push: %v", err)
	}
	if err := VerifySIG0(wire, key); err != nil {
		t.Fatalf("full-zone push should self-verify: %v", err)
	}
}
