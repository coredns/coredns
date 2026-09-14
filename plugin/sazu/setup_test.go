package sazu

import (
	"path/filepath"
	"testing"

	"github.com/coredns/caddy"
)

func TestParseSazu(t *testing.T) {
	tests := []struct {
		input          string
		shouldErr      bool
		wantZones      []string
		wantInsecure   bool
		wantRequireSig bool
		wantDBPath     string
	}{
		{
			input:     `sazu example.org.`,
			wantZones: []string{"example.org."},
		},
		{
			input: `sazu example.org. {
				rate_limit 10
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				rate_limit ten 100
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				rate_limit -1 100
			}`,
			shouldErr: true,
		},
		{
			input:     `sazu example.org. example.net.`,
			wantZones: []string{"example.org.", "example.net."},
		},
		{
			input: `sazu example.org. {
				insecure_skip_chain_validation
			}`,
			wantZones:    []string{"example.org."},
			wantInsecure: true,
		},
		{
			input: `sazu example.org. {
				db /tmp/sazu-test.db
			}`,
			wantZones:  []string{"example.org."},
			wantDBPath: "/tmp/sazu-test.db",
		},
		{
			input: `sazu example.org. {
				insecure_skip_chain_validation
				db /tmp/sazu-test.db
			}`,
			wantZones:    []string{"example.org."},
			wantInsecure: true,
			wantDBPath:   "/tmp/sazu-test.db",
		},
		{
			input: `sazu example.org. {
				require_valid_rrsigs
			}`,
			wantZones:      []string{"example.org."},
			wantRequireSig: true,
		},
		{
			input: `sazu example.org. {
				require_valid_rrsigs extra
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				db
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				db /tmp/a /tmp/b
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				insecure_skip_chain_validation extra
			}`,
			shouldErr: true,
		},
		{
			input: `sazu example.org. {
				bogus
			}`,
			shouldErr: true,
		},
	}

	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		cfg, err := parseSazu(c)
		if tc.shouldErr {
			if err == nil {
				t.Errorf("test %d: expected an error, got none", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test %d: unexpected error: %v", i, err)
		}
		if len(cfg.zones) != len(tc.wantZones) {
			t.Fatalf("test %d: got zones %v, want %v", i, cfg.zones, tc.wantZones)
		}
		for j, z := range tc.wantZones {
			if cfg.zones[j] != z {
				t.Fatalf("test %d: zone %d = %q, want %q", i, j, cfg.zones[j], z)
			}
		}
		if cfg.insecureSkipChainValidation != tc.wantInsecure {
			t.Fatalf("test %d: insecureSkipChainValidation = %v, want %v", i, cfg.insecureSkipChainValidation, tc.wantInsecure)
		}
		if cfg.requireValidRRSIGs != tc.wantRequireSig {
			t.Fatalf("test %d: requireValidRRSIGs = %v, want %v", i, cfg.requireValidRRSIGs, tc.wantRequireSig)
		}
		if cfg.dbPath != tc.wantDBPath {
			t.Fatalf("test %d: dbPath = %q, want %q", i, cfg.dbPath, tc.wantDBPath)
		}
	}
}

func TestParseSazuRateLimitDefaultsAndOverride(t *testing.T) {
	c := caddy.NewTestController("dns", `sazu example.org.`)
	cfg, err := parseSazu(c)
	if err != nil {
		t.Fatalf("parseSazu: %v", err)
	}
	if cfg.fullPushesPerDay != DefaultFullPushesPerDay || cfg.differentialPushesPerDay != DefaultDifferentialPushesPerDay {
		t.Fatalf("expected default quotas (%d, %d) when rate_limit is omitted, got (%d, %d)",
			DefaultFullPushesPerDay, DefaultDifferentialPushesPerDay, cfg.fullPushesPerDay, cfg.differentialPushesPerDay)
	}

	c = caddy.NewTestController("dns", `sazu example.org. {
		rate_limit 10 100
	}`)
	cfg, err = parseSazu(c)
	if err != nil {
		t.Fatalf("parseSazu: %v", err)
	}
	if cfg.fullPushesPerDay != 10 || cfg.differentialPushesPerDay != 100 {
		t.Fatalf("expected overridden quotas (10, 100), got (%d, %d)", cfg.fullPushesPerDay, cfg.differentialPushesPerDay)
	}
}

func TestSetupWiresPluginAndOptionalDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sazu.db")
	input := `sazu example.org. {
		insecure_skip_chain_validation
		db ` + dbPath + `
	}`
	c := caddy.NewTestController("dns", input)
	if err := setup(c); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func TestSetupWithoutDBIsPurelyInMemory(t *testing.T) {
	c := caddy.NewTestController("dns", `sazu example.org.`)
	if err := setup(c); err != nil {
		t.Fatalf("setup: %v", err)
	}
}
