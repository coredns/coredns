package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestDOFallbackPreservesPrefetch(t *testing.T) {
	for _, mode := range []string{"verify", "immediate"} {
		t.Run(mode, func(t *testing.T) {
			c := New()
			c.preferPositive = true
			c.staleUpTo = time.Hour
			c.verifyStale = mode == "verify"
			c.pttl = 10 * time.Second
			var seconds atomic.Int64
			c.now = func() time.Time { return time.Unix(1800000000+seconds.Load(), 0) }
			var signedCalls, unsignedCalls atomic.Int32
			started := make(chan struct{}, 8)
			completed := make(chan struct{}, 8)
			unsignedDone := make(chan struct{}, 8)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
				m := new(dns.Msg)
				m.SetReply(q)
				if requestDO(q) {
					if signedCalls.Add(1) > 1 {
						started <- struct{}{}
						<-release
						completed <- struct{}{}
						return 0, nil
					}
				} else {
					ip := "192.0.2.1"
					if unsignedCalls.Add(1) > 1 {
						ip = "192.0.2.2"
						defer func() { unsignedDone <- struct{}{} }()
					}
					m.Answer = []dns.RR{test.A("probe.example. 30 IN A " + ip)}
				}
				return 0, w.WriteMsg(m)
			})
			sharingServe(t, c, sharingQuery(false))
			seconds.Store(11)
			sharingServe(t, c, sharingQuery(true))
			key := hash("probe.example.", dns.TypeA, dns.ClassINET, false)
			holder, _ := c.pcache.Get(key)
			if !holder.do || holder.answering || holder.lastKnownGood == nil {
				t.Fatal("expected unsigned fallback below a DO1 non-answer")
			}
			fallback := holder.lastKnownGood
			current := holder
			t.Cleanup(func() {
				unblock()
				deadline := time.Now().Add(2 * time.Second)
				for (holder.refreshing.Load() || current.refreshing.Load() || fallback.refreshing.Load()) && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if holder.refreshing.Load() || current.refreshing.Load() || fallback.refreshing.Load() {
					t.Error("owned refresh did not finish")
				}
			})
			c.prefetch = 1
			c.percentage = 10
			seconds.Store(15) // The empty response has TTL5: DO1 TTL1, DO0 expired.
			if holder.ttl(c.now()) != 1 {
				t.Fatal("expected DO1 TTL1")
			}
			sharingServe(t, c, sharingQuery(true))
			waitForStaleSignal(t, started, "DO1 prefetch did not start")
			refreshed := sharingServe(t, c, sharingQuery(false))
			want := "probe.example.\t10\tIN\tA\t192.0.2.2"
			if mode == "immediate" {
				want = "probe.example.\t0\tIN\tA\t192.0.2.1"
			}
			if len(refreshed.Answer) != 1 || refreshed.Answer[0].String() != want {
				t.Fatalf("unsigned response = %v; want %s", refreshed, want)
			}
			waitForStaleRefresh(t, unsignedDone, fallback)
			current, _ = c.pcache.Get(key)
			if current == holder || current.lastKnownGood == fallback || current.lastKnownGood.do {
				t.Fatal("fallback was not independently replaced")
			}
			fresh := sharingServe(t, c, sharingQuery(false))
			if len(fresh.Answer) != 1 || fresh.Answer[0].String() != "probe.example.\t10\tIN\tA\t192.0.2.2" {
				t.Fatalf("fresh fallback = %v", fresh)
			}
			for range 3 {
				sharingServe(t, c, sharingQuery(true))
			}
			duplicate := false
			select {
			case <-started:
				duplicate = true
			case <-time.After(100 * time.Millisecond):
			}
			unblock()
			waitForStaleRefresh(t, completed, holder)
			deadline := time.Now().Add(time.Second)
			for current.refreshing.Load() && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if current.refreshing.Load() {
				t.Fatal("completion did not release the current holder")
			}
			if duplicate || signedCalls.Load() != 2 || unsignedCalls.Load() != 2 {
				t.Fatalf("duplicate=%t DO1=%d want2 DO0=%d want2", duplicate, signedCalls.Load(), unsignedCalls.Load())
			}
			// Completion must leave the shared lifecycle reusable, not permanently busy.
			sharingServe(t, c, sharingQuery(true))
			waitForStaleSignal(t, started, "DO1 prefetch did not restart after completion")
			waitForStaleRefresh(t, completed, current)
			if signedCalls.Load() != 3 {
				t.Fatalf("DO1 calls after completion = %d; want3", signedCalls.Load())
			}
			t.Log("one in-flight DO1 fetch across fallback publication; completion releases guard")
		})
	}
}

func TestFallbackHolderRefreshFailureRecheck(t *testing.T) {
	now := time.Unix(1800000000, 0)
	response := new(dns.Msg)
	response.SetReply(sharingQuery(true))
	response.Extra = []dns.RR{test.A("ns.example. 30 IN A 192.0.2.53")}
	original := newItem(response, now, 5*time.Second)
	original.do = true
	answer := new(dns.Msg)
	answer.SetReply(sharingQuery(false))
	answer.Answer = []dns.RR{test.A("probe.example. 30 IN A 192.0.2.1")}
	fallback := newItem(answer, now, 30*time.Second)
	original.lastKnownGood = fallback
	if !original.beginRefresh(now, 30*time.Second) {
		t.Fatal("initial refresh not acquired")
	}
	current := original
	for range 3 {
		previous := current
		nextAnswer := newItem(answer.Copy(), now, 30*time.Second)
		current = previous.withLastKnownGood(nextAnswer)
		if previous.lastKnownGood == nextAnswer || current.lastKnownGood != nextAnswer || nextAnswer.lastKnownGood != nil {
			t.Fatal("fallback publication mutated a holder or created a history chain")
		}
		if current.beginRefresh(now, 30*time.Second) {
			t.Fatal("replacement admitted an extra refresh")
		}
		if current.stored != original.stored || current.origTTL != original.origTTL || !current.do || current.Freq != original.Freq {
			t.Fatal("retained acquisition changed")
		}
	}
	// The old goroutine completes after several immutable holder publications.
	original.endRefresh(now, 30*time.Second, false)
	deadline := now.Add(30 * time.Second)
	if current.refreshing.Load() || current.retryAfter.Load() == nil || !current.retryAfter.Load().Equal(deadline) {
		t.Fatal("failed completion was not visible through the current holder")
	}
	current = current.withLastKnownGood(newItem(answer.Copy(), now, 30*time.Second))
	if current.beginRefresh(now.Add(29*time.Second), 30*time.Second) {
		t.Fatal("holder replacement lost failure recheck deadline")
	}
	if !current.beginRefresh(deadline, 30*time.Second) || original.beginRefresh(deadline, 30*time.Second) {
		t.Fatal("deadline must admit exactly one refresh across all holders")
	}
	current.endRefresh(deadline, 30*time.Second, true)
	if original.refreshing.Load() || original.retryAfter.Load() != nil {
		t.Fatal("successful completion did not clear shared lifecycle")
	}
	if original.Extra[0].String() != "ns.example.\t30\tIN\tA\t192.0.2.53" || original.lastKnownGood != fallback {
		t.Fatal("original payload or fallback was mutated")
	}
	// A new acquisition is independent, even while the old one is refreshing.
	if !original.beginRefresh(deadline, 0) {
		t.Fatal("old lifecycle not reusable")
	}
	acquired := newItem(response.Copy(), deadline, 5*time.Second)
	if !acquired.beginRefresh(deadline, 30*time.Second) {
		t.Fatal("new acquisition inherited old refresh state")
	}
	acquired.endRefresh(deadline, 30*time.Second, false)
	if !original.refreshing.Load() || original.retryAfter.Load() != nil {
		t.Fatal("new acquisition changed the old lifecycle")
	}
	original.endRefresh(deadline, 0, true)
}
