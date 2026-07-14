package dnslkg

import (
	"path/filepath"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

// TestParse validates Corefile parsing without opening a database.
func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"bare", `dnslkg`, false},
		{"path arg", `dnslkg /tmp/lkg.db`, false},
		{"too many args", `dnslkg a b`, true},
		{"path block", `dnslkg {
			path /tmp/lkg.db
		}`, false},
		{"ttl", `dnslkg db {
			ttl 15s
		}`, false},
		{"zero ttl", `dnslkg db {
			ttl 0s
		}`, false},
		{"bad ttl", `dnslkg db {
			ttl notaduration
		}`, true},
		{"negative ttl", `dnslkg db {
			ttl -5s
		}`, true},
		{"include", `dnslkg db {
			include ^example\.org\.$ ^example\.com\.$
		}`, false},
		{"exclude", `dnslkg db {
			exclude ^internal\.$
		}`, false},
		{"empty include", `dnslkg db {
			include
		}`, true},
		{"bad regex", `dnslkg db {
			include (
		}`, true},
		{"unknown property", `dnslkg db {
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
	if d.path != "dnslkg.db" {
		t.Errorf("Expected default path %q, got %q", "dnslkg.db", d.path)
	}
	if d.ttl != defaultTTL {
		t.Errorf("Expected default ttl %v, got %v", defaultTTL, d.ttl)
	}
	if len(d.include) != 0 || len(d.exclude) != 0 {
		t.Errorf("Expected no include/exclude patterns by default")
	}
}

func TestParsePatterns(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg db {
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

// TestSetup exercises the full setup path, including opening the SQLite store,
// and closes the store afterwards so the temporary file can be removed.
func TestSetup(t *testing.T) {
	db := filepath.Join(t.TempDir(), "lkg.db")
	c := caddy.NewTestController("dns", `dnslkg `+db)

	if err := setup(c); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Retrieve the registered handler and close its store to release the file.
	cfg := dnsserver.GetConfig(c)
	if len(cfg.Plugin) == 0 {
		t.Fatal("Expected a plugin to be registered")
	}
	h := cfg.Plugin[len(cfg.Plugin)-1](nil)
	d, ok := h.(*DnsLKG)
	if !ok {
		t.Fatalf("Expected *DnsLKG, got %T", h)
	}
	if err := d.store.close(); err != nil {
		t.Errorf("Closing store: %v", err)
	}
}

func TestSetupInvalid(t *testing.T) {
	c := caddy.NewTestController("dns", `dnslkg a b`)
	if err := setup(c); err == nil {
		t.Fatal("Expected an error for invalid config")
	}
}
