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
	lastSwept    time.Time
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
	now := r.now()
	sweepIfDue(&r.lastSwept, r.window, now, r.full, r.differential)
	return slidingWindowAllow(bucket, normalizeZone(zone), limit, r.window, now)
}

// slidingWindowAllow is the check-and-record primitive both RateLimiter
// (per-zone, daily) and IPRateLimiter (per-source-IP, per-minute) share:
// does key have fewer than limit recorded events within window of now?
// If so, record one and return true. Compacts bucket[key] in place,
// dropping anything outside the window, before counting -- safe because
// the write index never runs ahead of the read index (only entries
// already read are ever re-appended). Callers are responsible for their
// own locking; this touches the map and slice directly with none of its
// own.
func slidingWindowAllow(bucket map[string][]time.Time, key string, limit int, window time.Duration, now time.Time) bool {
	cutoff := now.Add(-window)
	kept := bucket[key][:0]
	for _, t := range bucket[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		bucket[key] = kept
		return false
	}
	bucket[key] = append(kept, now)
	return true
}

// sweepIfDue garbage-collects every bucket's stale entries at most once
// per window, updating *lastSwept when it does. Without this, a sliding-
// window limiter's memory grows with the number of *distinct keys ever
// seen*, never shrinking -- for RateLimiter, one entry per distinct zone
// name a caller has attempted (even a first-contact attempt that never
// verified, since garbage zone names are free to vary); for IPRateLimiter,
// one entry per distinct source address (including a single one-shot
// spoofed UDP packet, which by construction is never seen from the same
// address twice). An attacker who can cheaply vary that identity on every
// attempt could otherwise grow this map's memory usage without bound, one
// entry per attempt, forever. Sweeping at most once per window keeps
// memory instead bounded by "how many distinct keys were active in
// roughly the last window," which is bounded by the attacker's own
// sustained traffic rate -- an ordinary, expected property of a rate
// limiter, not a new attack surface it introduces.
func sweepIfDue(lastSwept *time.Time, window time.Duration, now time.Time, buckets ...map[string][]time.Time) {
	if now.Sub(*lastSwept) < window {
		return
	}
	*lastSwept = now
	cutoff := now.Add(-window)
	for _, bucket := range buckets {
		for key, times := range bucket {
			stillLive := false
			for _, t := range times {
				if t.After(cutoff) {
					stillLive = true
					break
				}
			}
			if !stillLive {
				delete(bucket, key)
			}
		}
	}
}
