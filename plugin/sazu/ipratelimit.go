package sazu

import (
	"net"
	"sync"
	"time"
)

// DefaultIPUpdatesPerMinute is §12's starting number for the global,
// per-source-IP flood/scan throttle (the ERR_RATE_LIMITED status code
// this document's own earlier notes named but hadn't built yet). This is
// deliberately distinct from, and enforced independently of,
// RateLimiter's per-zone daily push quota: that quota is keyed by zone
// name, so it does nothing against an attacker who varies the *target*
// zone name on each attempt -- every never-before-seen name starts a
// fresh, empty quota bucket, which is exactly the shape of "probe many
// candidate zones, looking for one whose delegation or DNSSEC state is
// exploitable" the per-zone quota alone cannot bound. Only an
// address-keyed limit, independent of which zone(s) an address targets,
// actually closes that gap.
const DefaultIPUpdatesPerMinute = 30

// IPRateLimiter enforces a global cap on UPDATE transactions per source
// IP address, over a rolling 1-minute window -- a much shorter,
// coarser-grained throttle than RateLimiter's per-zone daily quota,
// deliberately: this is a flood/scan guard reacting on the timescale an
// actual scan happens on, not a churn guard reacting on the timescale a
// legitimate customer's own DNS hygiene happens on.
//
// Not persisted, for the same reason RateLimiter isn't: a restart resets
// every address's counter, a deliberate, conservative simplification --
// the failure mode is "briefly too permissive after a restart," never
// "a legitimate address locked out," and this is a purely advisory
// abuse guard, not something correctness depends on.
type IPRateLimiter struct {
	mu        sync.Mutex
	perMinute int
	window    time.Duration
	seen      map[string][]time.Time
	now       func() time.Time // overridable in tests
	lastSwept time.Time
}

// NewIPRateLimiter returns an IPRateLimiter allowing perMinute UPDATE
// transactions per source IP address, per rolling 1-minute window.
func NewIPRateLimiter(perMinute int) *IPRateLimiter {
	return &IPRateLimiter{
		perMinute: perMinute,
		window:    time.Minute,
		seen:      make(map[string][]time.Time),
		now:       time.Now,
	}
}

// Allow reports whether another UPDATE transaction from remoteAddr (as
// returned by dns.ResponseWriter.RemoteAddr().String() -- a "host:port"
// or bare host/IP string across every transport this plugin serves:
// UDP, TCP, and HTTPS/HTTP3's DoHWriter) is within quota, recording it
// immediately if so, atomically under one lock.
//
// Deliberately called before any authentication check in serveUpdate,
// unlike RateLimiter.Allow: this bounds raw attempt *volume* from an
// address regardless of whether any given attempt is well-formed, valid,
// or for a real zone, which is exactly the property a flood/scan guard
// needs -- an attacker's failed probes cost this server real CPU and, for
// a first-contact attempt, a real outbound network walk, whether or not
// SIG(0) or anything else about the attempt ever turns out to verify.
//
// Precisely because this is called for every attempt regardless of
// validity, the address it's keyed by can be pure one-shot garbage --
// most notably a spoofed source address on a single UDP packet, which by
// construction is never seen from the same address twice. See
// sweepIfDue's own doc comment for why this periodically garbage-collects
// r.seen rather than letting it grow with every distinct address ever
// observed.
func (r *IPRateLimiter) Allow(remoteAddr string) bool {
	ip := sourceIP(remoteAddr)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	sweepIfDue(&r.lastSwept, r.window, now, r.seen)
	return slidingWindowAllow(r.seen, ip, r.perMinute, r.window, now)
}

// sourceIP strips the port from a "host:port" remote address, falling
// back to the address exactly as given if it isn't in that form, so
// per-connection port churn from one client (a new ephemeral port on
// every UDP/TCP/HTTPS request) doesn't fragment its rate-limit bucket
// into one entry per port instead of one per address.
func sourceIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
