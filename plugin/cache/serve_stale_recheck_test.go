package cache

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type staleRecheckClock struct {
	base   time.Time
	offset atomic.Int64
}

func newStaleRecheckClock() *staleRecheckClock {
	return &staleRecheckClock{base: time.Now()}
}

func (c *staleRecheckClock) Now() time.Time {
	return c.base.Add(time.Duration(c.offset.Load()))
}

func (c *staleRecheckClock) Set(offset time.Duration) {
	c.offset.Store(int64(offset))
}

func TestServeStaleFailureRecheckImmediate(t *testing.T) {
	tests := []struct {
		name          string
		prime         plugin.Handler
		expectedRcode int
	}{
		{name: "positive", prime: ttlBackend(1), expectedRcode: dns.RcodeSuccess},
		{name: "negative", prime: nxDomainBackend(1), expectedRcode: dns.RcodeNameError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newStaleRecheckClock()
			c := New()
			c.now = clock.Now
			c.minpttl = 0
			c.minnttl = 0
			c.staleUpTo = time.Hour
			c.staleTTL = 30 * time.Second
			c.staleRecheck = 30 * time.Second
			c.Next = tc.prime

			req := new(dns.Msg)
			req.SetQuestion("cached.org.", dns.TypeA)
			serveStaleRecheckRequest(t, c, req)
			item := c.exists("cached.org.", dns.TypeA, dns.ClassINET, false, false)
			if item == nil {
				t.Fatal("expected primed cache item")
			}

			clock.Set(2 * time.Second)
			var calls atomic.Int32
			started := make(chan struct{}, 2)
			completed := make(chan struct{}, 2)
			release := make(chan struct{}, 2)
			defer close(release)
			failure := servFailBackend(30)
			c.Next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				calls.Add(1)
				started <- struct{}{}
				<-release
				rcode, err := failure.ServeDNS(ctx, w, r)
				completed <- struct{}{}
				return rcode, err
			})

			msg := serveStaleRecheckRequest(t, c, req)
			if msg.Rcode != tc.expectedRcode {
				t.Fatalf("expected stale rcode %d, got %d", tc.expectedRcode, msg.Rcode)
			}
			waitForStaleSignal(t, started, "background refresh did not start")
			release <- struct{}{}
			waitForStaleRefresh(t, completed, item)
			if got := calls.Load(); got != 1 {
				t.Fatalf("expected one refresh attempt, got %d", got)
			}

			msg = serveStaleRecheckRequest(t, c, req)
			if msg.Rcode != tc.expectedRcode {
				t.Fatalf("expected stale rcode %d during recheck, got %d", tc.expectedRcode, msg.Rcode)
			}
			if item.refreshing.Load() {
				t.Fatal("failure recheck did not suppress a new refresh")
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("expected refresh to be suppressed during recheck, got %d attempts", got)
			}

			clock.Set(33 * time.Second)
			serveStaleRecheckRequest(t, c, req)
			waitForStaleSignal(t, started, "refresh did not restart after recheck elapsed")
			release <- struct{}{}
			waitForStaleRefresh(t, completed, item)
			if got := calls.Load(); got != 2 {
				t.Fatalf("expected refresh after recheck elapsed, got %d attempts", got)
			}
			if got := c.exists("cached.org.", dns.TypeA, dns.ClassINET, false, false).Rcode; got != tc.expectedRcode {
				t.Fatalf("failed refresh replaced stale state: expected rcode %d, got %d", tc.expectedRcode, got)
			}
		})
	}
}

func TestServeStaleFailureRecheckVerify(t *testing.T) {
	clock := newStaleRecheckClock()
	c := New()
	c.now = clock.Now
	c.minpttl = 0
	c.minnttl = 0
	c.staleUpTo = time.Hour
	c.verifyStale = true
	c.staleRecheck = 30 * time.Second
	c.Next = ttlBackend(1)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	serveStaleRecheckRequest(t, c, req)
	clock.Set(2 * time.Second)

	var calls atomic.Int32
	failure := servFailBackend(30)
	c.Next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls.Add(1)
		return failure.ServeDNS(ctx, w, r)
	})

	serveStaleRecheckRequest(t, c, req)
	serveStaleRecheckRequest(t, c, req)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one verify during failure recheck, got %d", got)
	}

	clock.Set(33 * time.Second)
	serveStaleRecheckRequest(t, c, req)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected verify after failure recheck elapsed, got %d", got)
	}
}

func TestServeStaleFailureRecheckRejectsInvalidRefresh(t *testing.T) {
	modes := []struct {
		name   string
		verify bool
	}{
		{name: "immediate"},
		{name: "verify", verify: true},
	}
	invalidResponses := []struct {
		name  string
		build func(*dns.Msg) *dns.Msg
	}{
		{
			name: "truncated",
			build: func(req *dns.Msg) *dns.Msg {
				m := new(dns.Msg)
				m.SetReply(req)
				m.Truncated = true
				m.Answer = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.20")}
				return m
			},
		},
		{
			name: "mismatched question",
			build: func(req *dns.Msg) *dns.Msg {
				m := new(dns.Msg)
				m.SetReply(req)
				m.Question[0].Name = "other.org."
				m.Answer = []dns.RR{test.A("other.org. 60 IN A 192.0.2.20")}
				return m
			},
		},
		{
			name: "expired RRSIG",
			build: func(req *dns.Msg) *dns.Msg {
				m := new(dns.Msg)
				m.SetReply(req)
				m.SetEdns0(4096, true)
				m.Answer = []dns.RR{
					test.A("cached.org. 60 IN A 192.0.2.20"),
					test.RRSIG("cached.org. 60 IN RRSIG A 8 2 60 20160521031301 20160421031301 12051 cached.org. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				}
				return m
			},
		},
	}

	for _, mode := range modes {
		for _, invalid := range invalidResponses {
			t.Run(mode.name+"/"+invalid.name, func(t *testing.T) {
				clock := newStaleRecheckClock()
				c := New()
				c.now = clock.Now
				c.minpttl = 0
				c.staleUpTo = time.Hour
				c.verifyStale = mode.verify
				c.staleTTL = 30 * time.Second
				c.staleRecheck = 30 * time.Second
				c.Next = ttlBackend(1)

				req := new(dns.Msg)
				req.SetQuestion("cached.org.", dns.TypeA)
				req.SetEdns0(4096, true)
				serveStaleRecheckRequest(t, c, req)
				item := c.exists("cached.org.", dns.TypeA, dns.ClassINET, true, false)
				if item == nil {
					t.Fatal("expected primed cache item")
				}
				clock.Set(2 * time.Second)

				var calls atomic.Int32
				completed := make(chan struct{}, 1)
				c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) (int, error) {
					calls.Add(1)
					err := w.WriteMsg(invalid.build(req))
					completed <- struct{}{}
					return dns.RcodeSuccess, err
				})

				msg := serveStaleRecheckRequest(t, c, req)
				assertStaleRecheckAddress(t, msg)
				waitForStaleRefresh(t, completed, item)
				if retryAfter := item.retryAfter.Load(); retryAfter == nil || !retryAfter.After(clock.Now()) {
					t.Fatal("invalid refresh did not start the failure recheck interval")
				}

				msg = serveStaleRecheckRequest(t, c, req)
				assertStaleRecheckAddress(t, msg)
				if got := calls.Load(); got != 1 {
					t.Fatalf("expected invalid refresh to be suppressed during recheck, got %d attempts", got)
				}
			})
		}
	}
}

func TestServeStaleFailureRecheckVerifyTimeoutCoalesces(t *testing.T) {
	clock := newStaleRecheckClock()
	c := New()
	c.now = clock.Now
	c.minpttl = 0
	c.minnttl = 0
	c.staleUpTo = time.Hour
	c.verifyStale = true
	c.verifyStaleTimeout = 10 * time.Millisecond
	c.staleRecheck = 30 * time.Second
	c.Next = ttlBackend(1)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	serveStaleRecheckRequest(t, c, req)
	item := c.exists("cached.org.", dns.TypeA, dns.ClassINET, false, false)
	clock.Set(2 * time.Second)

	var calls atomic.Int32
	started := make(chan struct{}, 2)
	completed := make(chan struct{}, 2)
	release := make(chan struct{})
	failure := servFailBackend(30)
	c.Next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		rcode, err := failure.ServeDNS(ctx, w, r)
		completed <- struct{}{}
		return rcode, err
	})

	serveStaleRecheckRequest(t, c, req)
	waitForStaleSignal(t, started, "background verify did not start")
	serveStaleRecheckRequest(t, c, req)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected concurrent stale request to share the in-flight verify, got %d attempts", got)
	}

	close(release)
	waitForStaleRefresh(t, completed, item)
	serveStaleRecheckRequest(t, c, req)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected failed background verify to start recheck delay, got %d attempts", got)
	}

	clock.Set(33 * time.Second)
	serveStaleRecheckRequest(t, c, req)
	waitForStaleRefresh(t, completed, item)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected a new verify after recheck elapsed, got %d attempts", got)
	}
}

func TestServeStaleFailureRecheckDisabledPreservesVerifyBehavior(t *testing.T) {
	clock := newStaleRecheckClock()
	c := New()
	c.now = clock.Now
	c.minpttl = 0
	c.minnttl = 0
	c.staleUpTo = time.Hour
	c.verifyStale = true
	c.Next = ttlBackend(1)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	serveStaleRecheckRequest(t, c, req)
	clock.Set(2 * time.Second)

	var calls atomic.Int32
	failure := servFailBackend(30)
	c.Next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls.Add(1)
		return failure.ServeDNS(ctx, w, r)
	})

	serveStaleRecheckRequest(t, c, req)
	serveStaleRecheckRequest(t, c, req)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected disabled failure recheck to preserve per-request verify, got %d attempts", got)
	}
}

func serveStaleRecheckRequest(t *testing.T, c *Cache, req *dns.Msg) *dns.Msg {
	t.Helper()
	recorder := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := c.ServeDNS(context.Background(), recorder, req.Copy()); err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}
	if recorder.Msg == nil {
		t.Fatal("ServeDNS did not write a response")
	}
	return recorder.Msg
}

func assertStaleRecheckAddress(t *testing.T, msg *dns.Msg) {
	t.Helper()
	if msg.Truncated || len(msg.Answer) != 1 {
		t.Fatalf("expected one complete stale answer, got %#v", msg)
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok || a.A.String() != "127.0.0.53" {
		t.Fatalf("expected stale address 127.0.0.53, got %v", msg.Answer)
	}
	if a.Hdr.Ttl != 30 {
		t.Fatalf("expected stale TTL 30, got %d", a.Hdr.Ttl)
	}
}

func waitForStaleRefresh(t *testing.T, completed <-chan struct{}, item *item) {
	t.Helper()
	waitForStaleSignal(t, completed, "background refresh did not complete")
	deadline := time.Now().Add(2 * time.Second)
	for item.refreshing.Load() {
		if time.Now().After(deadline) {
			t.Fatal("background refresh did not release the cache item")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForStaleSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(failure)
	}
}
