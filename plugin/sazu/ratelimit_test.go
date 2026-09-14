package sazu

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToTheLimitThenRefuses(t *testing.T) {
	r := NewRateLimiter(2, 3)
	for i := 0; i < 2; i++ {
		if !r.Allow("example.org.", true) {
			t.Fatalf("full push %d: expected within quota", i)
		}
	}
	if r.Allow("example.org.", true) {
		t.Fatalf("expected the 3rd full push to exceed the quota of 2")
	}
}

func TestRateLimiterTracksFullAndDifferentialIndependently(t *testing.T) {
	r := NewRateLimiter(1, 1)
	if !r.Allow("example.org.", true) {
		t.Fatalf("expected the first full push within quota")
	}
	if !r.Allow("example.org.", false) {
		t.Fatalf("expected a differential push to have its own, independent quota")
	}
	if r.Allow("example.org.", true) {
		t.Fatalf("expected the full quota to already be exhausted")
	}
	if r.Allow("example.org.", false) {
		t.Fatalf("expected the differential quota to already be exhausted")
	}
}

func TestRateLimiterTracksZonesIndependently(t *testing.T) {
	r := NewRateLimiter(1, 1)
	if !r.Allow("a.example.", true) {
		t.Fatalf("expected a.example.'s first full push within quota")
	}
	if !r.Allow("b.example.", true) {
		t.Fatalf("expected b.example.'s quota to be entirely independent of a.example.'s")
	}
}

func TestRateLimiterSlidesTheWindow(t *testing.T) {
	r := NewRateLimiter(1, 1)
	now := time.Now()
	r.now = func() time.Time { return now }
	if !r.Allow("example.org.", true) {
		t.Fatalf("expected the first push within quota")
	}
	if r.Allow("example.org.", true) {
		t.Fatalf("expected the quota to be exhausted immediately after")
	}

	r.now = func() time.Time { return now.Add(24*time.Hour + time.Second) }
	if !r.Allow("example.org.", true) {
		t.Fatalf("expected quota to be available again once the first push aged out of the rolling 24h window")
	}
}

func TestRateLimiterIsCaseInsensitiveOnZoneName(t *testing.T) {
	r := NewRateLimiter(1, 1)
	if !r.Allow("Example.ORG.", true) {
		t.Fatalf("expected the first push within quota")
	}
	if r.Allow("example.org.", true) {
		t.Fatalf("expected the same zone under different casing to share one quota")
	}
}
