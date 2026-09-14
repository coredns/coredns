package sazu

import (
	"fmt"
	"testing"
	"time"
)

// TestIPRateLimiterSweepsStaleAddressesOutOfMemory proves the specific
// memory-growth gap sweepIfDue closes: many distinct, never-repeated
// addresses (standing in for one-shot spoofed UDP source addresses, or
// simply many real distinct clients over time) must not accumulate in
// r.seen forever -- once their entries have aged out of the window, and
// enough time has passed for a sweep to become due, the map shrinks back
// down rather than retaining one entry per address ever observed.
func TestIPRateLimiterSweepsStaleAddressesOutOfMemory(t *testing.T) {
	r := NewIPRateLimiter(30)
	now := time.Now()
	r.now = func() time.Time { return now }

	const distinctAddresses = 500
	for i := 0; i < distinctAddresses; i++ {
		// Vary two octets so every i really is a distinct IP address
		// (sourceIP strips the port, so varying only the port would
		// collapse them all onto one address).
		addr := fmt.Sprintf("203.0.%d.%d:40000", i/256, i%256)
		if !r.Allow(addr) {
			t.Fatalf("attempt %d: expected within quota (each address is distinct)", i)
		}
	}
	if got := len(r.seen); got != distinctAddresses {
		// Sanity check on the test's own setup, not the behavior under
		// test: confirms every address really did get its own entry
		// before any sweep, and that a sweep isn't yet due.
		t.Fatalf("expected %d map entries before any sweep is due, got %d", distinctAddresses, got)
	}

	// Advance well past the window and one more sweep interval -- every
	// one of those entries is now stale.
	r.now = func() time.Time { return now.Add(2 * r.window) }

	// The next Allow call (for yet another, brand new address) is what
	// triggers the periodic sweep -- see sweepIfDue's doc comment.
	if !r.Allow("198.51.100.1:9999") {
		t.Fatalf("expected the triggering call itself to be allowed")
	}

	if got := len(r.seen); got != 1 {
		t.Fatalf("expected the sweep to have removed all %d stale entries, leaving only the one that just triggered it; got %d entries remaining", distinctAddresses, got)
	}
}

// TestRateLimiterSweepsStaleZoneNamesOutOfMemory is the same proof for
// RateLimiter's per-zone maps: a garbage zone name that only ever
// verifies once (or fails-open before ever reaching RateLimiter.Allow
// again) must not permanently occupy memory either.
func TestRateLimiterSweepsStaleZoneNamesOutOfMemory(t *testing.T) {
	r := NewRateLimiter(5, 50)
	now := time.Now()
	r.now = func() time.Time { return now }

	const distinctZones = 500
	for i := 0; i < distinctZones; i++ {
		zone := fmt.Sprintf("zone-%d.example.", i)
		if !r.Allow(zone, true) {
			t.Fatalf("attempt %d: expected within quota (each zone is distinct)", i)
		}
	}
	if got := len(r.full); got != distinctZones {
		t.Fatalf("expected %d map entries before any sweep is due, got %d", distinctZones, got)
	}

	r.now = func() time.Time { return now.Add(2 * r.window) }
	if !r.Allow("trigger.example.", true) {
		t.Fatalf("expected the triggering call itself to be allowed")
	}

	if got := len(r.full); got != 1 {
		t.Fatalf("expected the sweep to have removed all %d stale zone entries, leaving only the one that just triggered it; got %d entries remaining", distinctZones, got)
	}
}

func TestSweepIfDueOnlyRunsOncePerWindow(t *testing.T) {
	bucket := map[string][]time.Time{}
	now := time.Now()
	var lastSwept time.Time

	bucket["stale"] = []time.Time{now.Add(-time.Hour)}
	sweepIfDue(&lastSwept, time.Minute, now, bucket)
	if _, ok := bucket["stale"]; ok {
		t.Fatalf("expected the first sweep to remove the already-stale entry")
	}

	// Immediately add another stale entry and sweep again right away --
	// too soon for a second sweep (less than one window since lastSwept).
	bucket["stale2"] = []time.Time{now.Add(-time.Hour)}
	sweepIfDue(&lastSwept, time.Minute, now, bucket)
	if _, ok := bucket["stale2"]; !ok {
		t.Fatalf("expected no sweep to run again before a full window has passed")
	}

	// Now advance past the window -- due again.
	sweepIfDue(&lastSwept, time.Minute, now.Add(2*time.Minute), bucket)
	if _, ok := bucket["stale2"]; ok {
		t.Fatalf("expected the second sweep, once due, to remove the stale entry")
	}
}
