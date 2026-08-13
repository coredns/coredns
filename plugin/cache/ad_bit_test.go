package cache

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// TestCacheADBitNotPartitioned guards the fix for issue #6642.
//
// The cache key is hashed on qname, qtype, qclass, the DO bit and the CD bit, but
// deliberately NOT on the AD bit (AD must not partition the cache). This means
// a single cache entry serves both AD-requesting and non-AD-requesting clients,
// so the AD bit returned to a client must be derived per request from the
// authentication status of the cached answer, never frozen from whichever query
// happened to populate the entry first.
//
// Historically (reported on v1.11.1) the order mattered: a first query without
// the AD bit cached the answer with AD cleared, and a later query WITH the AD
// bit could no longer get AD=1 back. The reverse order worked. This test pins
// both directions so the asymmetry cannot regress.
func TestCacheADBitNotPartitioned(t *testing.T) {
	// Forward order: +noad first (populates cache), then +ad on the same entry.
	// The upstream answer is authenticated, so the +ad query must receive AD=1.
	t.Run("noad_then_ad", func(t *testing.T) {
		c := New()
		h := &adBitHandler{}
		c.Next = h

		// First query: AD not requested, DO not set -> miss, populates cache.
		noad := new(dns.Msg)
		noad.SetQuestion("invent.example.org.", dns.TypeA)
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, noad)
		if rec.Msg.AuthenticatedData {
			t.Errorf("first query did not request AD, expected AuthenticatedData=false, got true")
		}
		if c.pcache.Len() != 1 {
			t.Fatalf("expected first query to populate one cache entry, got %d", c.pcache.Len())
		}

		// Second query: AD requested, DO not set -> hit on the same key.
		// Must reflect the authenticated answer with AD=1.
		ad := new(dns.Msg)
		ad.SetQuestion("invent.example.org.", dns.TypeA)
		ad.AuthenticatedData = true
		rec = dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, ad)
		if h.calls != 1 {
			t.Fatalf("expected second query to be served from cache, backend calls=%d", h.calls)
		}
		if !rec.Msg.AuthenticatedData {
			t.Errorf("second query requested AD on an authenticated cached answer, expected AuthenticatedData=true, got false")
		}
	})

	// Reverse order: +ad first, then +noad. This direction already worked; we
	// pin it so a fix for the forward case never breaks it.
	t.Run("ad_then_noad", func(t *testing.T) {
		c := New()
		h := &adBitHandler{}
		c.Next = h

		// First query: AD requested -> AD=1 expected.
		ad := new(dns.Msg)
		ad.SetQuestion("invent.example.org.", dns.TypeA)
		ad.AuthenticatedData = true
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, ad)
		if !rec.Msg.AuthenticatedData {
			t.Errorf("first query requested AD on an authenticated answer, expected AuthenticatedData=true, got false")
		}
		if c.pcache.Len() != 1 {
			t.Fatalf("expected first query to populate one cache entry, got %d", c.pcache.Len())
		}

		// Second query: AD not requested -> AD must be cleared for this client.
		noad := new(dns.Msg)
		noad.SetQuestion("invent.example.org.", dns.TypeA)
		rec = dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, noad)
		if h.calls != 1 {
			t.Fatalf("expected second query to be served from cache, backend calls=%d", h.calls)
		}
		if rec.Msg.AuthenticatedData {
			t.Errorf("second query did not request AD, expected AuthenticatedData=false, got true")
		}
	})
}

type adBitHandler struct {
	calls int
}

func (h *adBitHandler) Name() string { return "adBitHandler" }

func (h *adBitHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	h.calls++

	m := new(dns.Msg)
	m.SetReply(r)
	m.AuthenticatedData = true
	m.Answer = []dns.RR{test.A("invent.example.org. 60 IN A 192.0.2.1")}
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}
