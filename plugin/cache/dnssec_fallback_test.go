package cache

import (
	"context"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

// A DO=1 upgrade can produce a non-answer while a still useful DO=0
// answer is retained by prefer_positive. Retention must not pretend that
// the old answer is DO-capable, nor lose it for subsequent DO=0 clients.
func TestDOUpgradeRetainsUnsignedLastKnownGood(t *testing.T) {
	c := New()
	c.preferPositive = true
	c.staleUpTo = time.Hour
	calls := 0
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, q *dns.Msg) (int, error) {
		calls++
		m := sharingResponse(q, "positive", false)
		if requestDO(q) {
			m.Answer = nil
		}
		return 0, w.WriteMsg(m)
	})
	sharingServe(t, c, sharingQuery(false))
	got := sharingServe(t, c, sharingQuery(true))
	if len(got.Answer) != 0 {
		t.Fatal("DO=1 used the unsigned answer")
	}
	got = sharingServe(t, c, sharingQuery(false))
	if len(got.Answer) != 1 || got.Answer[0].Header().Rrtype != dns.TypeA {
		t.Fatalf("lost unsigned last-known-good: %s", got)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	got = sharingServe(t, c, sharingQuery(true))
	if len(got.Answer) != 0 {
		t.Fatal("DO=1 used retained unsigned answer")
	}
}
