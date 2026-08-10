package cache

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestNewItemPreservesMonotonicClock(t *testing.T) {
	now := time.Now()
	if reflect.DeepEqual(now, now.Round(0)) {
		t.Fatal("time.Now did not include a monotonic clock reading")
	}
	i := newItem(new(dns.Msg), now, time.Minute)

	// DeepEqual compares the complete time representation, including its
	// monotonic clock reading. Time.Equal intentionally ignores that detail.
	if !reflect.DeepEqual(i.stored, now) {
		t.Fatalf("stored time = %v; want original time %v", i.stored, now)
	}
}

// TestCacheDoesNotSynthesizeAA guards issue #6185.
//
// A cached answer that came from a non-authoritative upstream must not gain the
// AA bit when it is served from the cache. Historically toMsg set AA
// unconditionally, so the same query answered AA=0 on a miss and AA=1 on a hit.
// Both directions are pinned here.
func TestCacheDoesNotSynthesizeAA(t *testing.T) {
	c := New()
	c.Next = BackendHandler() // replies with Authoritative unset

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	// Miss: the answer is passed through, AA must stay 0.
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req)
	if rec.Msg.Authoritative {
		t.Fatalf("cache miss: expected AA=0 from a non-authoritative backend, got AA=1")
	}

	// Hit: the same answer is rebuilt from the cached item, AA must still be 0.
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req)
	if rec.Msg.Authoritative {
		t.Errorf("cache hit: expected AA=0, the cached answer was not authoritative, got AA=1")
	}
}
