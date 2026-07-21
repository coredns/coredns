// Package dnslkg implements a plugin that serves the Last Known Good (LKG)
// DNS answer whenever an upstream returns a negative (NXDOMAIN / NODATA) or
// error (e.g. SERVFAIL) response, or fails to respond at all.
//
// Unlike the cache plugin's serve_stale option - which only kicks in when the
// upstream is considered unhealthy - dnslkg falls back to the last successful
// answer whenever the current response is a failure. This covers the class of
// failure that caused large scale outages such as the 2025 AWS DNS incident: a
// healthy-but-misconfigured upstream that starts returning NXDOMAIN / NODATA
// for names that previously resolved.
//
// Answers are kept in a simple bounded in-memory store (see Store). Persistence
// is deliberately left out of the default backend but the Store interface makes
// it straightforward to add later.
package dnslkg

import (
	"context"
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

	store Store
	// maxEntries bounds the in-memory store. 0 selects defaultMaxEntries.
	maxEntries int
	// maxAge, when > 0, is the maximum age of an entry that may be served.
	maxAge time.Duration
	// ttl, when > 0, is the TTL (in seconds) stamped on every record of an
	// answer served from the LKG store. Keeping it short makes clients re-query
	// frequently so that a recovered upstream is picked up quickly.
	ttl time.Duration
	// fb selects which upstream failures cause the LKG answer to be served.
	fb fallbackSet
	// fallbackTimeout, when > 0, is a soft deadline: the query is always
	// forwarded upstream, but if no answer arrives within this window the LKG
	// answer is served immediately (the late upstream answer still refreshes
	// the store). Only active when fb.timeout is set.
	fallbackTimeout time.Duration
	// matcher selects which query names are tracked.
	matcher *nameMatcher
}

// fallbackSet records which failure classes trigger an LKG fallback.
type fallbackSet struct {
	nxdomain bool // authoritative NXDOMAIN
	nodata   bool // NOERROR with no matching-type records
	timeout  bool // no usable response / transport failure / soft-timeout
	serverr  bool // SERVFAIL and other error rcodes
}

// allFallbacks returns the default (fully permissive) trigger set.
func allFallbacks() fallbackSet {
	return fallbackSet{nxdomain: true, nodata: true, timeout: true, serverr: true}
}

// defaultTTL is the TTL used for served LKG answers when none is configured.
const defaultTTL = 30 * time.Second

// Name implements the plugin.Handler interface.
func (d *DnsLKG) Name() string { return "dnslkg" }

// upstreamResult carries the outcome of the forwarded query between goroutines.
type upstreamResult struct {
	rcode int
	err   error
}

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

	// Soft-deadline ("verify") mode: forward upstream but fall back to LKG if it
	// does not answer within fallbackTimeout.
	if d.fallbackTimeout > 0 && d.fb.timeout {
		done := make(chan upstreamResult, 1)
		go func() {
			rc, e := plugin.NextOrFailure(d.Name(), d.Next, ctx, nw, r)
			done <- upstreamResult{rc, e}
		}()

		select {
		case res := <-done:
			return d.handleUpstream(w, r, nw, res.rcode, res.err, qname, qtype, server)
		case <-time.After(d.fallbackTimeout):
			if d.serveFallback(w, r, qname, qtype, server) {
				// The late upstream answer still refreshes the store.
				go d.refreshInBackground(done, nw, qname, qtype, server)
				return dns.RcodeSuccess, nil
			}
			// No LKG entry to fall back to; wait for the real answer.
			res := <-done
			return d.handleUpstream(w, r, nw, res.rcode, res.err, qname, qtype, server)
		}
	}

	rcode, err := plugin.NextOrFailure(d.Name(), d.Next, ctx, nw, r)
	return d.handleUpstream(w, r, nw, rcode, err, qname, qtype, server)
}

// handleUpstream classifies the upstream response and either stores it, serves
// an LKG fallback, or passes it through, according to the configured triggers.
func (d *DnsLKG) handleUpstream(w dns.ResponseWriter, r *dns.Msg, nw *nonwriter.Writer, rcode int, err error, qname string, qtype uint16, server string) (int, error) {
	if err == nil && nw.Msg != nil {
		ty, _ := response.Typify(nw.Msg, time.Now().UTC())
		switch ty {
		case response.NoError:
			// A good answer: remember it as the last known good and pass it on.
			d.storeAnswer(qname, qtype, nw.Msg, server)
			w.WriteMsg(nw.Msg)
			return rcode, nil
		case response.NameError:
			if d.fb.nxdomain && d.serveFallback(w, r, qname, qtype, server) {
				return dns.RcodeSuccess, nil
			}
		case response.NoData:
			if d.fb.nodata && d.serveFallback(w, r, qname, qtype, server) {
				return dns.RcodeSuccess, nil
			}
		case response.OtherError:
			if d.fb.serverr && d.serveFallback(w, r, qname, qtype, server) {
				return dns.RcodeSuccess, nil
			}
		}
		// No LKG fallback available (or not configured); pass the response on.
		w.WriteMsg(nw.Msg)
		return rcode, nil
	}

	// The upstream failed to produce a usable message at all.
	if d.fb.timeout && d.serveFallback(w, r, qname, qtype, server) {
		return dns.RcodeSuccess, nil
	}

	// Nothing to fall back to; propagate the failure unchanged.
	if nw.Msg != nil {
		w.WriteMsg(nw.Msg)
		return rcode, nil
	}
	return rcode, err
}

// storeAnswer records m as the last known good answer for qname/qtype.
func (d *DnsLKG) storeAnswer(qname string, qtype uint16, m *dns.Msg, server string) {
	if perr := d.store.Put(qname, qtype, m); perr != nil {
		log.Warningf("Failed to store LKG answer for %q: %v", qname, perr)
		return
	}
	storedResponses.WithLabelValues(server).Inc()
}

// serveFallback writes the stored LKG answer for qname/qtype to w and returns
// true if one was available.
func (d *DnsLKG) serveFallback(w dns.ResponseWriter, r *dns.Msg, qname string, qtype uint16, server string) bool {
	m := d.serveLKG(qname, qtype, r)
	if m == nil {
		return false
	}
	servedResponses.WithLabelValues(server).Inc()
	w.WriteMsg(m)
	return true
}

// refreshInBackground waits for a late upstream answer (after a soft-timeout
// fallback was already served) and stores it if it is good. It never touches
// the client's ResponseWriter.
func (d *DnsLKG) refreshInBackground(done <-chan upstreamResult, nw *nonwriter.Writer, qname string, qtype uint16, server string) {
	res := <-done
	if res.err != nil || nw.Msg == nil {
		return
	}
	if ty, _ := response.Typify(nw.Msg, time.Now().UTC()); ty == response.NoError {
		d.storeAnswer(qname, qtype, nw.Msg, server)
	}
}

// serveLKG returns a response built from the stored last known good answer for
// qname/qtype, or nil if none is available. The returned message is adapted to
// the incoming request (id, question) and its TTLs are normalised.
func (d *DnsLKG) serveLKG(qname string, qtype uint16, r *dns.Msg) *dns.Msg {
	cached, storedAt, err := d.store.Get(qname, qtype)
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

// shouldTrack reports whether qname is subject to LKG handling, per the
// configured include/exclude rules (see nameMatcher).
func (d *DnsLKG) shouldTrack(qname string) bool {
	if d.matcher == nil {
		return true
	}
	return d.matcher.tracked(qname)
}
