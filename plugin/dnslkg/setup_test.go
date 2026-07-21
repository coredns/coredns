package dnslkg

import (
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

// TestParse validates Corefile parsing without opening the store.
func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"bare", `dnslkg`, false},
		{"positional arg rejected", `dnslkg /tmp/lkg.db`, true},
		{"empty block", `dnslkg {
		}`, false},
		{"fallback_on subset", `dnslkg {
			fallback_on nodata
		}`, false},
		{"fallback_on multiple", `dnslkg {
			fallback_on nxdomain nodata timeout error
		}`, false},
		{"fallback_on all alias", `dnslkg {
			fallback_on all
		}`, false},
		{"fallback_on none alias", `dnslkg {
			fallback_on none
		}`, false},
		{"fallback_on empty", `dnslkg {
			fallback_on
		}`, true},
		{"fallback_on unknown", `dnslkg {
			fallback_on bogus
		}`, true},
		{"max_age", `dnslkg {
			max_age 24h
		}`, false},
		{"max_age negative", `dnslkg {
			max_age -1h
		}`, true},
		{"fallback_timeout", `dnslkg {
			fallback_timeout 200ms
		}`, false},
		{"max_entries", `dnslkg {
			max_entries 500
		}`, false},
		{"bad max_entries", `dnslkg {
			max_entries notanumber
		}`, true},
		{"zero max_entries", `dnslkg {
			max_entries 0
		}`, true},
		{"ttl", `dnslkg {
			ttl 15s
		}`, false},
		{"zero ttl", `dnslkg {
			ttl 0s
		}`, false},
		{"bad ttl", `dnslkg {
			ttl notaduration
		}`, true},
		{"negative ttl", `dnslkg {
			ttl -5s
		}`, true},
		{"include wildcard", `dnslkg {
			include *.example.org *.example.com
		}`, false},
		{"exclude wildcard", `dnslkg {
			exclude *.internal.example.org
		}`, false},
		{"empty include", `dnslkg {
			include
		}`, true},
		{"bad pattern mid-label wildcard", `dnslkg {
			include a*.example.org
		}`, true},
		{"unknown property", `dnslkg {
			bogus 1
		}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			_, err := parse(c)
			if tc.shouldErr && err == nil {
				t.Fatalf("Expected error, got none")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestParseDefaults(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if d.maxEntries != 0 {
		t.Errorf("Expected default maxEntries 0 (store applies default), got %d", d.maxEntries)
	}
	if d.maxAge != 0 {
		t.Errorf("Expected default maxAge 0, got %v", d.maxAge)
	}
	if d.fallbackTimeout != 0 {
		t.Errorf("Expected default fallbackTimeout 0, got %v", d.fallbackTimeout)
	}
	if d.ttl != defaultTTL {
		t.Errorf("Expected default ttl %v, got %v", defaultTTL, d.ttl)
	}
	if d.fb != allFallbacks() {
		t.Errorf("Expected all fallback triggers enabled by default, got %+v", d.fb)
	}
	if !d.shouldTrack("anything.example.org.") {
		t.Errorf("Expected all names tracked by default")
	}
}

func TestParseFallbackOn(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		fallback_on nodata
	}`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	want := fallbackSet{nodata: true}
	if d.fb != want {
		t.Errorf("Expected only nodata trigger, got %+v", d.fb)
	}
}

func TestParseMaxAgeAndTimeout(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		max_age 12h
		fallback_timeout 250ms
	}`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if d.maxAge != 12*time.Hour {
		t.Errorf("Expected maxAge 12h, got %v", d.maxAge)
	}
	if d.fallbackTimeout != 250*time.Millisecond {
		t.Errorf("Expected fallbackTimeout 250ms, got %v", d.fallbackTimeout)
	}
}

func TestParseMaxEntries(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		max_entries 250
	}`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if d.maxEntries != 250 {
		t.Errorf("Expected maxEntries 250, got %d", d.maxEntries)
	}
}

func TestParsePatterns(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		include *.example.com
		exclude *.internal.example.com
		include api.internal.example.com
	}`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Most-specific-wins across the three rules.
	if !d.shouldTrack("www.example.com.") {
		t.Error("Expected www.example.com. to be tracked")
	}
	if d.shouldTrack("db.internal.example.com.") {
		t.Error("Expected db.internal.example.com. to be excluded")
	}
	if !d.shouldTrack("api.internal.example.com.") {
		t.Error("Expected api.internal.example.com. to be re-included")
	}
	if d.shouldTrack("other.org.") {
		t.Error("Expected other.org. to be untracked (allow-list mode)")
	}
}

// TestSetup exercises the full setup path and closes the store afterwards.
func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		max_entries 100
		fallback_on nxdomain nodata
	}`)

	if err := setup(c); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	if len(cfg.Plugin) == 0 {
		t.Fatal("Expected a plugin to be registered")
	}
	h := cfg.Plugin[len(cfg.Plugin)-1](nil)
	d, ok := h.(*DnsLKG)
	if !ok {
		t.Fatalf("Expected *DnsLKG, got %T", h)
	}
	if err := d.store.Close(); err != nil {
		t.Errorf("Closing store: %v", err)
	}
}

func TestSetupInvalid(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg a b`)
	if err := setup(c); err == nil {
		t.Fatal("Expected an error for invalid config")
	}
}