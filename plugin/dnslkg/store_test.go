package dnslkg

import (
	"path/filepath"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func newTestStore(t *testing.T) *store {
	t.Helper()
	s, err := newStore(filepath.Join(t.TempDir(), "lkg.db"))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	t.Cleanup(func() { s.close() })
	return s
}

func msgWith(name string, qtype uint16, answer ...dns.RR) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	m.Response = true
	m.Answer = answer
	return m
}

func TestStorePutGet(t *testing.T) {
	s := newTestStore(t)

	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, storedAt, err := s.get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected a stored message, got nil")
	}
	if storedAt.IsZero() {
		t.Error("Expected a non-zero stored time")
	}
	if len(got.Answer) != 1 || got.Answer[0].String() != m.Answer[0].String() {
		t.Errorf("Unexpected answer round-trip: %v", got.Answer)
	}
}

func TestStoreGetMissing(t *testing.T) {
	s := newTestStore(t)

	got, _, err := s.get("missing.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for missing entry, got %v", got)
	}
}

func TestStoreKeyedByType(t *testing.T) {
	s := newTestStore(t)

	a := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.put("example.org.", dns.TypeA, a); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Different qtype must not collide with the stored A entry.
	got, _, err := s.get("example.org.", dns.TypeAAAA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for AAAA, got %v", got)
	}
}

func TestStoreOverwrite(t *testing.T) {
	s := newTestStore(t)

	first := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.put("example.org.", dns.TypeA, first); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	second := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 10.0.0.1"))
	if err := s.put("example.org.", dns.TypeA, second); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, _, err := s.get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(got.Answer) != 1 || got.Answer[0].(*dns.A).A.String() != "10.0.0.1" {
		t.Errorf("Expected overwritten answer 10.0.0.1, got %v", got.Answer)
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)

	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := s.delete("example.org.", dns.TypeA); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, _, err := s.get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil after delete, got %v", got)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lkg.db")

	s1, err := newStore(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s1.put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	s1.close()

	s2, err := newStore(path)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer s2.close()

	got, _, err := s2.get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected entry to persist across reopen, got nil")
	}
}
