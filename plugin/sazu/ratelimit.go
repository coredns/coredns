package sazu

import (
	"sync"
	"time"
)

// DefaultFullPushesPerDay and DefaultDifferentialPushesPerDay are §12's
// starting quota numbers: 5 full-zone pushes and 50 differential
// (push-update) pushes per zone, per rolling 24h window. Different limits
// for the two kinds because they have very different costs -- a full push
// re-verifies and re-signs an entire zone's worth of content (and, when
// it's also first contact, walks the chain of trust to the real DNS
// root), while a differential one only ever touches a handful of records.
const (
	DefaultFullPushesPerDay         = 5
	DefaultDifferentialPushesPerDay = 50
)

// RateLimiter enforces §12's per-zone push quotas over a rolling (not
// calendar-day) 24h window: FullPerDay full-zone pushes and
// DifferentialPerDay differential ones, tracked and enforced
// independently per zone.
//
// Not persisted: a restart resets every zone's quota, a deliberate,
// conservative simplification for this first cut -- the failure mode of
// losing quota history across a restart is "briefly too permissive,"
// never "a customer locked out of their own zone," and there is no
// operational reason yet to persist what is purely an abuse/churn guard
// rather than something a customer depends on for correctness.
type RateLimiter struct {
	mu           sync.Mutex
	fullPerDay   int
	diffPerDay   int
	window       time.Duration
	full         map[string][]time.Time
	differential map[string][]time.Time
	now          func() time.Time // overridable in tests
}

// NewRateLimiter returns a RateLimiter enforcing fullPerDay full-zone and
// diffPerDay differential pushes per zone, per rolling 24h window.
func NewRateLimiter(fullPerDay, diffPerDay int) *RateLimiter {
	return &RateLimiter{
		fullPerDay:   fullPerDay,
		diffPerDay:   diffPerDay,
		window:       24 * time.Hour,
		full:         make(map[string][]time.Time),
		differential: make(map[string][]time.Time),
		now:          time.Now,
	}
}

// Allow reports whether a push of the given kind (full or differential)
// for zone is within quota. If so, it records the push immediately as
// part of the same call -- check-and-record atomically under one lock, so
// two concurrent callers can't both observe "still room" for what is
// really only one remaining slot. Call this only for a push that has
// already passed authentication (SIG(0)): a quota is a bound on
// legitimate churn, not an identity check, and counting unauthenticated
// attempts against a zone's own quota would let anyone lock a customer
// out of their own zone with no proof of control over it at all.
func (r *RateLimiter) Allow(zone string, full bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, limit := r.differential, r.diffPerDay
	if full {
		bucket, limit = r.full, r.fullPerDay
	}

	zone = normalizeZone(zone)
	now := r.now()
	cutoff := now.Add(-r.window)

	// Compact in place, dropping anything outside the rolling window --
	// safe because the write index never runs ahead of the read index
	// (we only ever append what we've already read, never more).
	kept := bucket[zone][:0]
	for _, t := range bucket[zone] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		bucket[zone] = kept
		return false
	}
	bucket[zone] = append(kept, now)
	return true
}
