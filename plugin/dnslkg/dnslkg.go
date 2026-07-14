// Package dnslkg implements a plugin that serves the Last Known Good (LKG)
// DNS answer whenever an upstream returns a negative (NXDOMAIN / NODATA) or
// error (e.g. SERVFAIL) response, or fails to respond at all.
//
// Unlike the cache plugin's serve_stale option - which only kicks in when the
// upstream is considered unhealthy and which keeps its data in memory only -
// dnslkg persists every successful answer in an on-disk SQLite database. This
// lets CoreDNS keep serving the last good answer across restarts and, more
// importantly, when a healthy-but-misconfigured upstream starts returning
// NXDOMAIN / NODATA for names that previously resolved (the class of failure
// that caused large scale outages such as the 2025 AWS DNS incident).
package dnslkg

import (
	"context"
	"regexp"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("dnslkg")

// DnsLKG is the last-known-good plugin handler.
type DnsLKG struct {
	Next plugin.Handler

	store *store
	// path is the location of the on-disk SQLite database.
	path string
	// ttl, when > 0, is the TTL (in seconds) stamped on every record of an
	// answer served from the LKG store. Keeping it short makes clients re-query
	// frequently so that a recovered upstream is picked up quickly.
	ttl time.Duration
	// include and exclude are optional regular expressions matched against the
	// (lower-cased, fully-qualified) query name to select which names are
	// tracked. See shouldTrack for the exact semantics.
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

// defaultTTL is the TTL used for served LKG answers when none is configured.
const defaultTTL = 30 * time.Second

// Name implements the plugin.Handler interface.
func (d *DnsLKG) Name() string { return "dnslkg" }

// ServeDNS implements the plugin.Handler interface.
func (d *DnsLKG) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	qtype := state.QType()

	// Names we are not tracking bypass the plugin entirely.
	if !d.shouldTrack(qname) {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	server := metrics.WithServer(ctx)

	// Capture the downstream (upstream-facing) response without writing it to
	// the client yet, so we can decide whether to store it or replace it.
	nw := nonwriter.New(w)
	rcode, err := plugin.NextOrFailure(d.Name(), d.Next, ctx, nw, r)

	if err == nil && nw.Msg != nil {
		ty, _ := response.Typify(nw.Msg, time.Now().UTC())
		switch response.Classify(ty) {
		case response.Success:
			// A good answer: remember it as the last known good and pass it on.
			if perr := d.store.put(qname, qtype, nw.Msg); perr != nil {
				log.Warningf("Failed to store LKG answer for %q: %v", qname, perr)
			} else {
				storedResponses.WithLabelValues(server).Inc()
			}
			w.WriteMsg(nw.Msg)
			return rcode, nil
		case response.Denial, response.Error:
			// NXDOMAIN / NODATA / SERVFAIL: try to fall back to the LKG answer.
			if m := d.serveLKG(qname, qtype, r); m != nil {
				servedResponses.WithLabelValues(server).Inc()
				w.WriteMsg(m)
				return dns.RcodeSuccess, nil
			}
		}
		// No LKG fallback available; pass the original response through.
		w.WriteMsg(nw.Msg)
		return rcode, nil
	}

	// The upstream failed to produce a usable message at all; fall back to LKG.
	if m := d.serveLKG(qname, qtype, r); m != nil {
		servedResponses.WithLabelValues(server).Inc()
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	}

	// Nothing to fall back to; propagate the failure unchanged.
	if nw.Msg != nil {
		w.WriteMsg(nw.Msg)
		return rcode, nil
	}
	return rcode, err
}

// serveLKG returns a response built from the stored last known good answer for
// qname/qtype, or nil if none is available. The returned message is adapted to
// the incoming request (id, question) and its TTLs are normalised.
func (d *DnsLKG) serveLKG(qname string, qtype uint16, r *dns.Msg) *dns.Msg {
	cached, storedAt, err := d.store.get(qname, qtype)
	if err != nil {
		log.Warningf("Failed to read LKG answer for %q: %v", qname, err)
		return nil
	}
	if cached == nil {
		return nil
	}

	m := cached.Copy()
	m.Id = r.Id
	m.Question = r.Question
	m.Response = true
	m.Rcode = dns.RcodeSuccess

	d.stampTTL(m, storedAt)

	log.Debugf("Serving last known good answer for %q (stored %s ago)", qname, time.Since(storedAt).Truncate(time.Second))
	return m
}

// stampTTL rewrites the TTL of every record in m. When a ttl is configured it
// is used verbatim; otherwise the original TTL is decremented by the time the
// entry has spent in the store, with a small floor so the answer stays usable.
func (d *DnsLKG) stampTTL(m *dns.Msg, storedAt time.Time) {
	if d.ttl > 0 {
		setTTL(m, uint32(d.ttl.Seconds()))
		return
	}

	elapsed := uint32(time.Since(storedAt).Seconds())
	const floor = 5
	for _, rrs := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range rrs {
			if _, ok := rr.(*dns.OPT); ok {
				continue
			}
			orig := rr.Header().Ttl
			if orig > elapsed+floor {
				rr.Header().Ttl = orig - elapsed
			} else {
				rr.Header().Ttl = floor
			}
		}
	}
}

// setTTL sets a fixed TTL on every record in m (except OPT pseudo-records).
func setTTL(m *dns.Msg, ttl uint32) {
	for _, rrs := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range rrs {
			if _, ok := rr.(*dns.OPT); ok {
				continue
			}
			rr.Header().Ttl = ttl
		}
	}
}

// shouldTrack reports whether qname is subject to LKG handling.
//
//   - With no include/exclude patterns configured, every name is tracked.
//   - When include patterns are configured, a name must match at least one of
//     them to be tracked.
//   - A name matching any exclude pattern is never tracked, even if it also
//     matches an include pattern.
func (d *DnsLKG) shouldTrack(qname string) bool {
	if len(d.include) > 0 {
		matched := false
		for _, re := range d.include {
			if re.MatchString(qname) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, re := range d.exclude {
		if re.MatchString(qname) {
			return false
		}
	}
	return true
}
