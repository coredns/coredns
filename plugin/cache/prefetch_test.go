package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestPrefetch(t *testing.T) {
	tests := []struct {
		qname         string
		ttl           int
		prefetch      int
		verifications []verification
	}{
		{
			qname:    "hits.reset.example.org.",
			ttl:      80,
			prefetch: 1,
			verifications: []verification{
				{
					after:  0 * time.Second,
					answer: "hits.reset.example.org. 80 IN A 127.0.0.1",
					fetch:  true, // Initial fetch
				},
				{
					after:  73 * time.Second,
					answer: "hits.reset.example.org.  7 IN A 127.0.0.1",
					fetch:  true, // Triggers prefetch with 7 TTL (10% of 80 = 8 TTL threshold)
				},
				{
					after:  80 * time.Second,
					answer: "hits.reset.example.org. 73 IN A 127.0.0.2",
				},
			},
		},
		{
			qname:    "short.ttl.example.org.",
			ttl:      5,
			prefetch: 1,
			verifications: []verification{
				{
					after:  0 * time.Second,
					answer: "short.ttl.example.org. 5 IN A 127.0.0.1",
					fetch:  true,
				},
				{
					after:  1 * time.Second,
					answer: "short.ttl.example.org. 4 IN A 127.0.0.1",
				},
				{
					after:  4 * time.Second,
					answer: "short.ttl.example.org. 1 IN A 127.0.0.1",
					fetch:  true,
				},
				{
					after:  5 * time.Second,
					answer: "short.ttl.example.org. 4 IN A 127.0.0.2",
				},
			},
		},
		{
			qname:    "no.prefetch.example.org.",
			ttl:      30,
			prefetch: 0,
			verifications: []verification{
				{
					after:  0 * time.Second,
					answer: "no.prefetch.example.org. 30 IN A 127.0.0.1",
					fetch:  true,
				},
				{
					after:  15 * time.Second,
					answer: "no.prefetch.example.org. 15 IN A 127.0.0.1",
				},
				{
					after:  29 * time.Second,
					answer: "no.prefetch.example.org.  1 IN A 127.0.0.1",
				},
				{
					after:  30 * time.Second,
					answer: "no.prefetch.example.org. 30 IN A 127.0.0.2",
					fetch:  true,
				},
			},
		},
		{
			// tests whether cache prefetches with the do bit
			qname:    "do.prefetch.example.org.",
			ttl:      80,
			prefetch: 1,
			verifications: []verification{
				{
					after:  0 * time.Second,
					answer: "do.prefetch.example.org. 80 IN A 127.0.0.1",
					do:     true,
					fetch:  true,
				},
				{
					after:  73 * time.Second,
					answer: "do.prefetch.example.org.  7 IN A 127.0.0.1",
					do:     true,
					fetch:  true,
				},
				{
					after:  80 * time.Second,
					answer: "do.prefetch.example.org. 73 IN A 127.0.0.2",
					do:     true,
				},
				{
					// Should be 127.0.0.3 as 127.0.0.2 was the prefetch WITH do bit
					after:  80 * time.Second,
					answer: "do.prefetch.example.org. 80 IN A 127.0.0.3",
					fetch:  true,
				},
			},
		},
		{
			// tests whether cache prefetches with the cd bit
			qname:    "cd.prefetch.example.org.",
			ttl:      80,
			prefetch: 1,
			verifications: []verification{
				{
					after:  0 * time.Second,
					answer: "cd.prefetch.example.org. 80 IN A 127.0.0.1",
					cd:     true,
					fetch:  true,
				},
				{
					after:  73 * time.Second,
					answer: "cd.prefetch.example.org.  7 IN A 127.0.0.1",
					cd:     true,
					fetch:  true,
				},
				{
					after:  80 * time.Second,
					answer: "cd.prefetch.example.org. 73 IN A 127.0.0.2",
					cd:     true,
				},
				{
					// Should be 127.0.0.3 as 127.0.0.2 was the prefetch WITH cd bit
					after:  80 * time.Second,
					answer: "cd.prefetch.example.org. 80 IN A 127.0.0.3",
					fetch:  true,
				},
			},
		},
	}

	t0, err := time.Parse(time.RFC3339, "2018-01-01T14:00:00+00:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.qname, func(t *testing.T) {
			fetchc := make(chan struct{}, 1)

			c := New()
			c.Next = prefetchHandler(tt.qname, tt.ttl, fetchc)
			c.prefetch = tt.prefetch

			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			for _, v := range tt.verifications {
				c.now = func() time.Time { return t0.Add(v.after) }

				req := new(dns.Msg)
				req.SetQuestion(tt.qname, dns.TypeA)
				req.CheckingDisabled = v.cd
				req.SetEdns0(512, v.do)

				c.ServeDNS(context.TODO(), rec, req)
				if v.fetch {
					select {
					case <-fetchc:
						// Prefetch handler was called.
					case <-time.After(time.Second):
						t.Fatalf("After %s: want request to trigger a prefetch", v.after)
					}
				}
				if want, got := dns.RcodeSuccess, rec.Rcode; want != got {
					t.Errorf("After %s: want rcode %d, got %d", v.after, want, got)
				}
				if want, got := 1, len(rec.Msg.Answer); want != got {
					t.Errorf("After %s: want %d answer RR, got %d", v.after, want, got)
				}
				if want, got := test.A(v.answer).String(), rec.Msg.Answer[0].String(); want != got {
					t.Errorf("After %s: want answer %s, got %s", v.after, want, got)
				}
			}
		})
	}
}

type verification struct {
	after  time.Duration
	answer string
	do     bool
	cd     bool
	// fetch defines whether a request is sent to the next handler.
	fetch bool
}

// TestPrefetchDedup verifies that concurrent hits on a single cache item
// dispatch at most one prefetch goroutine, on both the serve_stale and
// shouldPrefetch paths. See https://github.com/coredns/coredns/issues/7904.
func TestPrefetchDedup(t *testing.T) {
	for _, tc := range []struct {
		name       string
		staleUpTo  time.Duration
		prefetch   int
		percentage int
		hitAt      time.Duration // all concurrent hits land here
	}{
		{name: "serve_stale", staleUpTo: time.Hour, hitAt: 110 * time.Second},
		{name: "prefetch", prefetch: 1, percentage: 50, hitAt: 70 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const N = 200
			var upstream atomic.Int32
			release := make(chan struct{})
			done := make(chan struct{}, N)

			c := New()
			c.staleUpTo = tc.staleUpTo
			c.prefetch = tc.prefetch
			c.percentage = tc.percentage
			c.duration = time.Minute
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				n := upstream.Add(1)
				if n > 1 {
					// Block the prefetch so all concurrent hits race against
					// the in-flight flag, not a completed refresh.
					<-release
					defer func() { done <- struct{}{} }()
				}
				m := new(dns.Msg)
				m.SetReply(r)
				m.Response = true
				m.Answer = []dns.RR{test.A("dedup.example.org. 100 IN A 127.0.0.1")}
				w.WriteMsg(m)
				return dns.RcodeSuccess, nil
			})

			t0 := time.Now().UTC()
			c.now = func() time.Time { return t0 }
			req := new(dns.Msg)
			req.SetQuestion("dedup.example.org.", dns.TypeA)
			c.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)
			if upstream.Load() != 1 {
				t.Fatalf("initial populate: want 1 upstream call, got %d", upstream.Load())
			}

			// Fire N concurrent hits while the item is prefetch-eligible.
			// Without dedup each would spawn its own prefetch goroutine;
			// with dedup only the CAS winner spawns one, and the remaining
			// N-1 hits serve from cache without touching upstream.
			c.now = func() time.Time { return t0.Add(tc.hitAt) }
			var wg sync.WaitGroup
			wg.Add(N)
			for range N {
				go func() {
					defer wg.Done()
					req := new(dns.Msg)
					req.SetQuestion("dedup.example.org.", dns.TypeA)
					c.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)
				}()
			}
			wg.Wait()

			close(release)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("prefetch goroutine never completed")
			}

			if got := upstream.Load(); got != 2 {
				t.Fatalf("want exactly 2 upstream calls (populate + 1 deduped prefetch), got %d", got)
			}
		})
	}
}

// prefetchHandler is a fake plugin implementation which returns a single A
// record with the given qname and ttl. The returned IP address starts at
// 127.0.0.1 and is incremented on every request.
func prefetchHandler(qname string, ttl int, fetchc chan struct{}) plugin.Handler {
	i := 0
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, _r *dns.Msg) (int, error) {
		i++
		m := new(dns.Msg)
		m.SetQuestion(qname, dns.TypeA)
		m.Response = true
		m.Answer = append(m.Answer, test.A(fmt.Sprintf("%s %d IN A 127.0.0.%d", qname, ttl, i)))

		w.WriteMsg(m)
		fetchc <- struct{}{}
		return dns.RcodeSuccess, nil
	})
}
