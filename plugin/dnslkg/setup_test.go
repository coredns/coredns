package dnslkg

import (
	"testing"

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
		{"max_entries", `dnslkg {
			max_entries 500
		}`, false},
		{"bad max_entries", `dnslkg {
			max_entries notanumber
		}`, true},
		{"zero max_entries", `dnslkg {
			max_entries 0
		}`, true},
		{"negative max_entries", `dnslkg {
			max_entries -5
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
		{"include", `dnslkg {
			include ^example\.org\.$ ^example\.com\.$
		}`, false},
		{"exclude", `dnslkg {
			exclude ^internal\.$
		}`, false},
		{"empty include", `dnslkg {
			include
		}`, true},
		{"bad regex", `dnslkg {
			include (
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
	if d.ttl != defaultTTL {
		t.Errorf("Expected default ttl %v, got %v", defaultTTL, d.ttl)
	}
	if len(d.include) != 0 || len(d.exclude) != 0 {
		t.Errorf("Expected no include/exclude patterns by default")
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
		include ^a\.$ ^b\.$
		include ^c\.$
		exclude ^x\.$
	}`)
	d, err := parse(c)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(d.include) != 3 {
		t.Errorf("Expected 3 include patterns, got %d", len(d.include))
	}
	if len(d.exclude) != 1 {
		t.Errorf("Expected 1 exclude pattern, got %d", len(d.exclude))
	}
}

// TestSetup exercises the full setup path and closes the store afterwards.
func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg {
		max_entries 100
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