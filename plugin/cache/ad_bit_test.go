package cache

import (
	"context"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

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
		if !h.requestedAD[0] {
			t.Errorf("cache refresh should ask upstream for AD when populating an AD-shared entry")
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

func TestCacheADBitStaleVerify(t *testing.T) {
	c := New()
	h := &adBitHandler{}
	c.Next = h
	c.staleUpTo = time.Hour
	c.verifyStale = true

	now := time.Now()
	c.now = func() time.Time { return now }

	req := new(dns.Msg)
	req.SetQuestion("invent.example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req)
	if c.pcache.Len() != 1 {
		t.Fatalf("expected first query to populate one cache entry, got %d", c.pcache.Len())
	}
	if !h.requestedAD[0] {
		t.Errorf("cache refresh should ask upstream for AD when populating an AD-shared entry")
	}

	now = now.Add(2 * time.Minute)

	ad := req.Copy()
	ad.AuthenticatedData = true
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, ad)
	if h.calls != 2 {
		t.Fatalf("expected stale verify to call backend, backend calls=%d", h.calls)
	}
	if !h.requestedAD[1] {
		t.Errorf("stale verify should ask upstream for AD when refreshing an AD-shared entry")
	}
	if !rec.Msg.AuthenticatedData {
		t.Errorf("AD-requesting client should receive AD from a refreshed authenticated answer, got false")
	}
}

func TestCacheADBitPrefetchRequestsAD(t *testing.T) {
	requestedAD := make(chan bool, 2)
	c := New()
	h := &adBitHandler{requestedADCh: requestedAD}
	c.Next = h
	c.prefetch = 1
	c.percentage = 100

	now := time.Now()
	c.now = func() time.Time { return now }

	req := new(dns.Msg)
	req.SetQuestion("invent.example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req)
	if got := <-requestedAD; !got {
		t.Errorf("cache refresh should ask upstream for AD when populating an AD-shared entry")
	}

	now = now.Add(time.Second)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req.Copy())

	select {
	case got := <-requestedAD:
		if !got {
			t.Errorf("prefetch should ask upstream for AD when refreshing an AD-shared entry")
		}
	case <-time.After(time.Second):
		t.Fatal("prefetch did not call backend")
	}
}

type adBitHandler struct {
	calls         int
	requestedAD   []bool
	requestedADCh chan bool
}

func (h *adBitHandler) Name() string { return "adBitHandler" }

func (h *adBitHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	h.calls++
	h.requestedAD = append(h.requestedAD, r.AuthenticatedData)
	if h.requestedADCh != nil {
		h.requestedADCh <- r.AuthenticatedData
	}

	m := new(dns.Msg)
	m.SetReply(r)
	state := request.Request{W: w, Req: r}
	m.AuthenticatedData = r.AuthenticatedData || state.Do()
	m.Answer = []dns.RR{test.A("invent.example.org. 60 IN A 192.0.2.1")}
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}
