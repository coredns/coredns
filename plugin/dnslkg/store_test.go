package dnslkg

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func newTestStore(t *testing.T) *memStore {
	t.Helper()
	return newMemStore(0)
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
	if err := s.Put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, storedAt, err := s.Get("example.org.", dns.TypeA)
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

	got, _, err := s.Get("missing.org.", dns.TypeA)
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
	if err := s.Put("example.org.", dns.TypeA, a); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Different qtype must not collide with the stored A entry.
	got, _, err := s.Get("example.org.", dns.TypeAAAA)
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
	if err := s.Put("example.org.", dns.TypeA, first); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	second := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 10.0.0.1"))
	if err := s.Put("example.org.", dns.TypeA, second); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, _, err := s.Get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(got.Answer) != 1 || got.Answer[0].(*dns.A).A.String() != "10.0.0.1" {
		t.Errorf("Expected overwritten answer 10.0.0.1, got %v", got.Answer)
	}
	// Overwriting must not grow the store.
	if len(s.entries) != 1 {
		t.Errorf("Expected a single entry after overwrite, got %d", len(s.entries))
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)

	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.Put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := s.Delete("example.org.", dns.TypeA); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, _, err := s.Get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil after delete, got %v", got)
	}
}

// TestStoreEviction verifies that the store never holds more than max entries
// and evicts the oldest-written entry first.
func TestStoreEviction(t *testing.T) {
	s := newMemStore(3)

	names := []string{"a.org.", "b.org.", "c.org.", "d.org."}
	for _, n := range names {
		m := msgWith(n, dns.TypeA, test.A(n+" 300 IN A 127.0.0.1"))
		if err := s.Put(n, dns.TypeA, m); err != nil {
			t.Fatalf("Put %q failed: %v", n, err)
		}
	}

	if len(s.entries) != 3 {
		t.Fatalf("Expected 3 entries after eviction, got %d", len(s.entries))
	}
	// a.org. was the oldest write and must have been evicted.
	if got, _, _ := s.Get("a.org.", dns.TypeA); got != nil {
		t.Error("Expected oldest entry a.org. to be evicted")
	}
	for _, n := range names[1:] {
		if got, _, _ := s.Get(n, dns.TypeA); got == nil {
			t.Errorf("Expected %q to still be present", n)
		}
	}
}

// TestStoreEvictionRefreshesOnWrite verifies that re-writing a key marks it
// most-recently-written, so it survives eviction over an older, untouched key.
func TestStoreEvictionRefreshesOnWrite(t *testing.T) {
	s := newMemStore(2)

	a := msgWith("a.org.", dns.TypeA, test.A("a.org. 300 IN A 127.0.0.1"))
	b := msgWith("b.org.", dns.TypeA, test.A("b.org. 300 IN A 127.0.0.1"))
	if err := s.Put("a.org.", dns.TypeA, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("b.org.", dns.TypeA, b); err != nil {
		t.Fatal(err)
	}

	// Refresh a.org. so it is now the most-recently-written.
	if err := s.Put("a.org.", dns.TypeA, a); err != nil {
		t.Fatal(err)
	}

	// Inserting c.org. should now evict b.org. (the oldest untouched write).
	c := msgWith("c.org.", dns.TypeA, test.A("c.org. 300 IN A 127.0.0.1"))
	if err := s.Put("c.org.", dns.TypeA, c); err != nil {
		t.Fatal(err)
	}

	if got, _, _ := s.Get("b.org.", dns.TypeA); got != nil {
		t.Error("Expected b.org. to be evicted")
	}
	if got, _, _ := s.Get("a.org.", dns.TypeA); got == nil {
		t.Error("Expected refreshed a.org. to survive eviction")
	}
}

func TestStoreCloseNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.Close(); err != nil {
		t.Errorf("Close should be a no-op, got: %v", err)
	}
}