package auto

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestAutoName(t *testing.T) {
	t.Parallel()
	a := Auto{}
	if a.Name() != "auto" {
		t.Errorf("Expected 'auto', got %s", a.Name())
	}
}

func TestAutoServeDNS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		qname        string
		qtype        uint16
		zones        []string
		expectedCode int
		shouldMatch  bool
	}{
		{
			name:         "valid A query",
			qname:        "test.example.org.",
			qtype:        dns.TypeA,
			zones:        []string{"example.org."},
			expectedCode: dns.RcodeServerFailure, // Zone exists but no data
			shouldMatch:  true,
		},
		{
			name:         "AXFR query refused",
			qname:        "test.example.org.",
			qtype:        dns.TypeAXFR,
			zones:        []string{"example.org."},
			expectedCode: dns.RcodeRefused,
			shouldMatch:  true,
		},
		{
			name:         "IXFR query refused",
			qname:        "test.example.org.",
			qtype:        dns.TypeIXFR,
			zones:        []string{"example.org."},
			expectedCode: dns.RcodeRefused,
			shouldMatch:  true,
		},
		{
			name:        "no matching zone",
			qname:       "test.notfound.org.",
			qtype:       dns.TypeA,
			zones:       []string{"example.org."},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := createTestAuto(tt.zones)

			m := new(dns.Msg)
			m.SetQuestion(tt.qname, tt.qtype)

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			ctx := context.Background()

			code, err := a.ServeDNS(ctx, rec, m)

			if !tt.shouldMatch {
				if err == nil {
					t.Errorf("Expected error for non-matching zone, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ServeDNS returned error: %v", err)
			}

			if tt.qtype == dns.TypeAXFR || tt.qtype == dns.TypeIXFR {
				if code != dns.RcodeRefused {
					t.Errorf("Expected RcodeRefused for %s, got %d", dns.TypeToString[tt.qtype], code)
				}
				return
			}

			if code != tt.expectedCode {
				t.Errorf("Expected code %d, got %d", tt.expectedCode, code)
			}
		})
	}
}

func TestAutoServeDNSZoneMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		origins      []string
		names        []string
		qname        string
		hasZone      bool
		shouldRefuse bool
	}{
		{
			name:         "exact zone match",
			origins:      []string{"example.org."},
			names:        []string{"example.org."},
			qname:        "test.example.org.",
			hasZone:      true,
			shouldRefuse: false,
		},
		{
			name:         "subdomain zone match",
			origins:      []string{"example.org."},
			names:        []string{"example.org."},
			qname:        "sub.test.example.org.",
			hasZone:      true,
			shouldRefuse: false,
		},
		{
			name:         "no origin match",
			origins:      []string{"other.org."},
			names:        []string{"example.org."},
			qname:        "test.example.org.",
			hasZone:      false,
			shouldRefuse: false,
		},
		{
			name:         "origin match but no name match",
			origins:      []string{"example.org."},
			names:        []string{"other.org."},
			qname:        "test.example.org.",
			hasZone:      false,
			shouldRefuse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &Auto{
				Zones: &Zones{
					Z:       make(map[string]*file.Zone),
					origins: tt.origins,
					names:   tt.names,
				},
				Next: nil,
			}

			for _, name := range tt.names {
				a.Z[name] = &file.Zone{}
			}

			m := new(dns.Msg)
			m.SetQuestion(tt.qname, dns.TypeA)

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			ctx := context.Background()

			code, err := a.ServeDNS(ctx, rec, m)

			if tt.hasZone {
				if err != nil {
					t.Errorf("Expected no error for zone match, got: %v", err)
				}
			} else {
				if tt.shouldRefuse {
					if code != dns.RcodeRefused {
						t.Errorf("Expected code %d, got %d", dns.RcodeRefused, code)
					}
				} else if err == nil {
					t.Errorf("Expected error for no zone match, got nil")
				}
			}
		})
	}
}

func TestAutoServeDNSNilZone(t *testing.T) {
	t.Parallel()

	a := &Auto{
		Zones: &Zones{
			Z:       make(map[string]*file.Zone),
			origins: []string{"example.org."},
			names:   []string{"example.org."},
		},
		Next: nil,
	}

	a.Z["example.org."] = nil

	m := new(dns.Msg)
	m.SetQuestion("test.example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := a.ServeDNS(ctx, rec, m)

	if code != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure for nil zone, got %d", code)
	}
	if err != nil {
		t.Errorf("Expected no error for nil zone, got: %v", err)
	}
}

func TestAutoServeDNSMissingZone(t *testing.T) {
	t.Parallel()

	a := &Auto{
		Zones: &Zones{
			Z:       make(map[string]*file.Zone),
			origins: []string{"example.org."},
			names:   []string{"example.org."},
		},
		Next: nil,
	}

	// Don't add the zone to the map to test the missing zone case

	m := new(dns.Msg)
	m.SetQuestion("test.example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := a.ServeDNS(ctx, rec, m)

	if code != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure for missing zone, got %d", code)
	}
	if err != nil {
		t.Errorf("Expected no error for missing zone, got: %v", err)
	}
}

// Helper functions for testing

func createTestAuto(zones []string) *Auto {
	a := &Auto{
		Zones: &Zones{
			Z:       make(map[string]*file.Zone),
			origins: zones,
			names:   zones,
		},
		Next: nil, // No next plugin for testing
	}

	// Initialize with empty zones for the tests
	for _, zone := range zones {
		a.Z[zone] = &file.Zone{}
	}

	return a
}
