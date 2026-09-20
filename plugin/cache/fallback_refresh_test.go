package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// A retained unsigned answer can refresh without replacing the newer DO1
// non-answer, or granting DNSSEC capability to the unsigned response.
func TestDOUnsignedFallbackRefresh(t *testing.T) {
	for _, mode := range []string{"verify", "immediate"} {
		for _, kind := range []string{"empty", "servfail", "nxdomain"} {
			for _, prefer := range []bool{false, true} {
				if !prefer && kind != "servfail" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/prefer=%t", mode, kind, prefer), func(t *testing.T) {
					c := New()
					c.preferPositive = prefer
					c.staleUpTo = time.Hour
					c.verifyStale = mode == "verify"
					base := time.Unix(1800000000, 0)
					var seconds atomic.Int64
					c.now = func() time.Time { return base.Add(time.Duration(seconds.Load()) * time.Second) }
					var calls atomic.Int32
					done := make(chan struct{})
					c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
						n := calls.Add(1)
						m := new(dns.Msg)
						m.SetReply(q)
						if n == 2 {
							if !requestDO(q) {
								t.Error("blocker was not acquired with DO")
							}
							if kind == "servfail" {
								m.Rcode = dns.RcodeServerFailure
							}
							if kind == "nxdomain" {
								m.Rcode = dns.RcodeNameError
								m.Ns = []dns.RR{test.SOA("example. 30 IN SOA ns.example. hostmaster.example. 1 60 60 600 30")}
							}
						} else {
							if requestDO(q) {
								t.Error("unsigned fallback refresh acquired DO")
							}
							ip := "192.0.2.1"
							if n >= 3 {
								ip = "192.0.2.2"
							}
							m.Answer = []dns.RR{test.A("probe.example. 30 IN A " + ip)}
						}
						if n == 3 {
							defer close(done)
						}
						return 0, w.WriteMsg(m)
					})
					check := func(m *dns.Msg, ttl uint32, ip string) {
						t.Helper()
						want := fmt.Sprintf("probe.example.\t%d\tIN\tA\t%s", ttl, ip)
						if m.Rcode != dns.RcodeSuccess || len(m.Answer) != 1 || m.Answer[0].String() != want || len(m.Ns) != 0 || len(m.Extra) != 0 {
							t.Fatalf("answer=%s want=%s", m, want)
						}
					}
					check(sharingServe(t, c, sharingQuery(false)), 30, "192.0.2.1")
					key := hash("probe.example.", dns.TypeA, dns.ClassINET, false)
					old, _ := c.pcache.Get(key)
					seconds.Store(31)
					blocked := sharingServe(t, c, sharingQuery(true))
					wantCode := dns.RcodeSuccess
					if kind == "servfail" {
						wantCode = dns.RcodeServerFailure
					}
					if kind == "nxdomain" {
						wantCode = dns.RcodeNameError
					}
					if blocked.Rcode != wantCode || len(blocked.Answer) != 0 || calls.Load() != 2 {
						t.Fatalf("blocker: calls=%d %s", calls.Load(), blocked)
					}
					holder, _ := c.pcache.Get(key)
					denial, _ := c.ncache.Get(key)
					// Fallback-only replacements share the retained acquisition's lifecycle.
					retry := base.Add(time.Minute)
					if kind == "empty" {
						holder.refreshing.Store(true)
						holder.retryAfter.Store(&retry)
					}
					seconds.Store(32)
					refreshed := sharingServe(t, c, sharingQuery(false))
					if mode == "verify" {
						check(refreshed, 30, "192.0.2.2")
					} else {
						check(refreshed, 0, "192.0.2.1")
					}
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Fatal("refresh did not complete")
					}
					deadline := time.Now().Add(time.Second)
					for old.refreshing.Load() && time.Now().Before(deadline) {
						time.Sleep(time.Millisecond)
					}
					if old.refreshing.Load() {
						t.Fatal("background refresh still active")
					}
					seconds.Store(33)
					check(sharingServe(t, c, sharingQuery(false)), 29, "192.0.2.2")
					if calls.Load() != 3 {
						t.Fatalf("successful refresh missed: calls=%d want3", calls.Load())
					}
					current, _ := c.pcache.Get(key)
					answer := current.answeringItem(request.Request{Req: sharingQuery(false)})
					if answer == nil || answer == old || answer.do || answer.matches(request.Request{Req: sharingQuery(true)}) {
						t.Fatal("fallback identity/capability invalid")
					}
					check(old.toMsgWithTTL(sharingQuery(false), 30, false, false), 30, "192.0.2.1")
					if kind == "empty" {
						if current == holder || !current.do || current.answering || current.stored != holder.stored || current.origTTL != holder.origTTL || holder.lastKnownGood != old {
							t.Fatal("DO1 holder mutated or lost")
						}
						if current.refreshState != holder.refreshState || !current.refreshing.Load() || current.retryAfter.Load() != &retry {
							t.Fatal("retained acquisition lost its shared refresh lifecycle")
						}
					} else {
						preserved, _ := c.ncache.Get(key)
						if preserved != denial || !preserved.do {
							t.Fatal("DO1 denial was discarded")
						}
					}
					signed := sharingServe(t, c, sharingQuery(true))
					if signed.Rcode != wantCode || len(signed.Answer) != 0 || calls.Load() != 3 {
						t.Fatalf("DO1 state not preserved: calls=%d %s", calls.Load(), signed)
					}
					if kind == "nxdomain" {
						want := "example.\t28\tIN\tSOA\tns.example. hostmaster.example. 1 60 60 600 30"
						if len(signed.Ns) != 1 || signed.Ns[0].String() != want {
							t.Fatalf("DO1 SOA: %v", signed.Ns)
						}
					}
					t.Logf("updated fallback TTL29 A192.0.2.2, DO1 %s retained, backend calls=%d", kind, calls.Load())
				})
			}
		}
	}
}
