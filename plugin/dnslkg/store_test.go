package dnslkg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func newTestStore(t *testing.T) *snapshotStore {
	t.Helper()
	s, err := newSnapshotStore(filepath.Join(t.TempDir(), "lkg.db"))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
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

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lkg.db")

	s1, err := newSnapshotStore(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s1.Put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	s1.Close()

	s2, err := newSnapshotStore(path)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer s2.Close()

	got, _, err := s2.Get("example.org.", dns.TypeA)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected entry to persist across reopen, got nil")
	}
}

// TestStoreDedupNoDirty verifies that re-storing an identical answer does not
// mark the store dirty (so a name that keeps resolving to the same value never
// triggers a disk write), while a changed answer does.
func TestStoreDedupNoDirty(t *testing.T) {
	s := newTestStore(t)

	m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 127.0.0.1"))
	if err := s.Put("example.org.", dns.TypeA, m); err != nil {
		t.Fatalf("First put failed: %v", err)
	}
	if err := s.flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if s.dirty {
		t.Fatal("Expected store to be clean after flush")
	}

	for i := 0; i < 100; i++ {
		if err := s.Put("example.org.", dns.TypeA, m); err != nil {
			t.Fatalf("Repeat put failed: %v", err)
		}
	}
	if s.dirty {
		t.Error("Expected identical repeat answers not to dirty the store")
	}

	// A changed answer must dirty the store.
	m2 := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A 10.0.0.1"))
	if err := s.Put("example.org.", dns.TypeA, m2); err != nil {
		t.Fatalf("Changed put failed: %v", err)
	}
	if !s.dirty {
		t.Error("Expected a changed answer to dirty the store")
	}
}

// TestStoreSnapshotStableSize verifies that overwriting the same key many times
// keeps the on-disk snapshot the size of the single live entry (no unbounded
// growth), and that the live entry survives a reopen.
func TestStoreSnapshotStableSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lkg.db")
	s, err := newSnapshotStore(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	for i := 0; i < 10000; i++ {
		ip := "10.0.0.1"
		if i%2 == 0 {
			ip = "10.0.0.2"
		}
		m := msgWith("example.org.", dns.TypeA, test.A("example.org. 300 IN A "+ip))
		if err := s.Put("example.org.", dns.TypeA, m); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}
	if err := s.flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	// One entry is well under 1 KiB; assert the snapshot did not grow per put.
	if fi.Size() > 1024 {
		t.Errorf("Expected snapshot to stay small for a single key, got %d bytes", fi.Size())
	}

	s.Close()
	s2, err := newSnapshotStore(path)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer s2.Close()
	if got, _, _ := s2.Get("example.org.", dns.TypeA); got == nil {
		t.Fatal("Expected the live entry to survive reopen")
	}
}

// TestStoreIgnoresCorruptSnapshot verifies that a corrupt snapshot file is
// ignored (store starts empty) rather than causing newStore to fail.
func TestStoreIgnoresCorruptSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lkg.db")

	if err := os.WriteFile(path, []byte("not a valid snapshot"), 0o600); err != nil {
		t.Fatalf("Write corrupt file failed: %v", err)
	}

	s, err := newSnapshotStore(path)
	if err != nil {
		t.Fatalf("Expected newStore to tolerate a corrupt snapshot, got: %v", err)
	}
	defer s.Close()
	if got, _, _ := s.Get("example.org.", dns.TypeA); got != nil {
		t.Errorf("Expected empty store from corrupt snapshot, got %v", got)
	}
}
