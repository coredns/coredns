package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// A failure acquired with DO can be specific to DNSSEC, for example from an
// upstream that only validates for DO clients, so it must not answer a client
// without DO, fresh or stale. That client queries upstream itself and its
// answer is cached without letting either response evict the other.
func TestDOSignedFailureDoesNotAnswerUnsigned(t *testing.T) {
	for _, rcode := range []int{dns.RcodeServerFailure, dns.RcodeNotImplemented} {
		for _, unsigned := range []string{"positive", "nxdomain", "servfail"} {
			for _, mode := range []string{"fresh", "immediate", "verify"} {
				t.Run(fmt.Sprintf("%s/%s/%s", dns.RcodeToString[rcode], unsigned, mode), func(t *testing.T) {
					c := New()
					var seconds atomic.Int64
					c.now = func() time.Time { return time.Unix(1800000000+seconds.Load(), 0) }
					if mode != "fresh" {
						c.staleUpTo = time.Hour
						c.verifyStale = mode == "verify"
					}
					var signedCalls, unsignedCalls atomic.Int32
					c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
						if requestDO(q) {
							signedCalls.Add(1)
							m := new(dns.Msg)
							m.SetRcode(q, rcode)
							return 0, w.WriteMsg(m)
						}
						unsignedCalls.Add(1)
						return 0, w.WriteMsg(sharingResponse(q, unsigned, false))
					})
					wantUnsigned := map[string]int{"positive": dns.RcodeSuccess, "nxdomain": dns.RcodeNameError, "servfail": dns.RcodeServerFailure}[unsigned]

					if got := sharingServe(t, c, sharingQuery(true)); got.Rcode != rcode || signedCalls.Load() != 1 {
						t.Fatalf("DO failure not served: calls=%d %s", signedCalls.Load(), got)
					}
					if mode != "fresh" {
						seconds.Store(10) // The failure (TTL 5) is now stale but within serve_stale.
					}
					for range 2 {
						if got := sharingServe(t, c, sharingQuery(false)); got.Rcode != wantUnsigned {
							t.Fatalf("client without DO got %s, want %s", dns.RcodeToString[got.Rcode], dns.RcodeToString[wantUnsigned])
						}
					}
					if unsignedCalls.Load() != 1 || signedCalls.Load() != 1 {
						t.Fatalf("unsigned answer not cached: DO0 calls=%d DO1 calls=%d", unsignedCalls.Load(), signedCalls.Load())
					}
					if mode != "fresh" {
						return
					}
					// DO clients keep their failure next to an unsigned answer. A DO
					// failure does not displace an unsigned denial, so it is fetched
					// again, and an unsigned failure also answers DO clients.
					wantSigned, wantSignedCalls := rcode, int32(1)
					switch unsigned {
					case "nxdomain":
						wantSignedCalls = 2
					case "servfail":
						wantSigned = dns.RcodeServerFailure
					}
					if got := sharingServe(t, c, sharingQuery(true)); got.Rcode != wantSigned || signedCalls.Load() != wantSignedCalls {
						t.Fatalf("DO client: calls=%d want%d %s", signedCalls.Load(), wantSignedCalls, got)
					}
					if got := sharingServe(t, c, sharingQuery(false)); got.Rcode != wantUnsigned || unsignedCalls.Load() != 1 {
						t.Fatalf("DO failure displaced the unsigned answer: calls=%d %s", unsignedCalls.Load(), got)
					}
				})
			}
		}
	}
}

// The late response guard keeps a slow DO=0 miss from replacing fresh DO=1
// data. A DO=1 failure is not data for that client and must not block it.
func TestDOLateUnsignedReplacesSignedFailure(t *testing.T) {
	for _, unsigned := range []string{"positive", "nxdomain"} {
		t.Run(unsigned, func(t *testing.T) {
			c := New()
			c.now = func() time.Time { return time.Unix(1800000000, 0) }
			started := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			var calls atomic.Int32
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
				calls.Add(1)
				if !requestDO(q) {
					close(started)
					<-release
					return 0, w.WriteMsg(sharingResponse(q, unsigned, false))
				}
				return 0, w.WriteMsg(sharingResponse(q, "servfail", true))
			})
			go func() {
				defer close(done)
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				c.ServeDNS(context.Background(), rec, sharingQuery(false))
			}()
			<-started
			if got := sharingServe(t, c, sharingQuery(true)); got.Rcode != dns.RcodeServerFailure {
				t.Fatalf("DO failure not served: %s", got)
			}
			close(release)
			<-done
			sharingAssertResponse(t, sharingServe(t, c, sharingQuery(false)), unsigned, false, 300)
			if calls.Load() != 2 {
				t.Fatalf("late unsigned answer not cached: calls=%d", calls.Load())
			}
			if unsigned == "positive" {
				if got := sharingServe(t, c, sharingQuery(true)); got.Rcode != dns.RcodeServerFailure || calls.Load() != 2 {
					t.Fatalf("DO failure lost: calls=%d %s", calls.Load(), got)
				}
			}
		})
	}
}

// Over the wire, a client without DO gets the upstream answer to its own query
// rather than a SERVFAIL that only DO queries received.
func TestDOSignedFailureWire(t *testing.T) {
	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			c := New()
			var seconds atomic.Int64
			c.now = func() time.Time { return time.Unix(1800000000+seconds.Load(), 0) }
			var calls atomic.Int32
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
				calls.Add(1)
				if requestDO(q) {
					return 0, w.WriteMsg(sharingResponse(q, "servfail", true))
				}
				return 0, w.WriteMsg(sharingResponse(q, "positive", false))
			})
			addr := sharingWireServer(t, network, c)
			client := &dns.Client{Net: network, Timeout: time.Second}
			for step, do := range []bool{true, false, true, false} {
				seconds.Store(int64(step))
				q := sharingQuery(do)
				got, _, err := client.Exchange(q, addr)
				if err != nil {
					t.Fatal(err)
				}
				kind, ttl := "servfail", uint32(0)
				if !do {
					kind, ttl = "positive", uint32(301-step) // Stored at step 1.
				}
				sharingAssertResponse(t, got, kind, false, ttl)
				if opt := got.IsEdns0(); got.Id != q.Id || opt == nil || opt.Do() != do {
					t.Fatalf("wire response %s", got)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("calls=%d want2", calls.Load())
			}
		})
	}
}
