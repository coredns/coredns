package tree

import (
	"testing"

	"github.com/miekg/dns"
)

// Test that Name() falls back to reading from the stored RRs when the cached name is empty.
func TestElemName_FallbackWhenCachedEmpty(t *testing.T) {
	rr, err := dns.NewRR("a.example. 3600 IN A 1.2.3.4")
	if err != nil {
		t.Fatalf("failed to create RR: %v", err)
	}

	// Build via newElem to ensure m is populated
	e := newElem(rr)
	got := e.Name()
	want := "a.example."
	if got != want {
		t.Fatalf("unexpected name; want %q, got %q", want, got)
	}

	// clear the cached name
	e.name = ""

	got = e.Name()
	want = "a.example."
	if got != want {
		t.Fatalf("unexpected name; want %q, got %q", want, got)
	}

	// clear the map
	e.m = make(map[uint16][]dns.RR, 0)

	got = e.Name()
	want = ""
	if got != want {
		t.Fatalf("unexpected name after clearing RR map; want %q, got %q", want, got)
	}
}
