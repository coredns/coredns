package cache

import (
	"reflect"
	"testing"
	"time"

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
