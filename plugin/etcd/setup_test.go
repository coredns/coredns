//go:build etcd

package etcd

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupEtcd(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedPath       string
		expectedEndpoint   []string
		expectedErrContent string // substring from the expected error. Empty for positive cases.
		username           string
		password           string
	}{
		// positive
		{
			`etcd`, false, "skydns", []string{"http://localhost:2379"}, "", "", "",
		},
		{
			`etcd {
	endpoint http://localhost:2379 http://localhost:3379 http://localhost:4379

}`, false, "skydns", []string{"http://localhost:2379", "http://localhost:3379", "http://localhost:4379"}, "", "", "",
		},
		{
			`etcd skydns.local {
	endpoint localhost:300
}
`, false, "skydns", []string{"localhost:300"}, "", "", "",
		},
		// negative
		{
			`etcd {
	endpoints localhost:300
}
`, true, "", []string{""}, "unknown property 'endpoints'", "", "",
		},
		// with valid credentials
		{
			`etcd {
			endpoint http://localhost:2379
			credentials username password
		}
			`, false, "skydns", []string{"http://localhost:2379"}, "", "username", "password",
		},
		// with credentials, missing password
		{
			`etcd {
			endpoint http://localhost:2379
			credentials username
		}
			`, true, "skydns", []string{"http://localhost:2379"}, "credentials requires 2 arguments", "username", "",
		},
		// with credentials, missing username and  password
		{
			`etcd {
			endpoint http://localhost:2379
			credentials
		}
			`, true, "skydns", []string{"http://localhost:2379"}, "Wrong argument count", "", "",
		},
		// with custom min-lease-ttl
		{
			`etcd {
			endpoint http://localhost:2379
			min-lease-ttl 60
		}
			`, false, "skydns", []string{"http://localhost:2379"}, "", "", "",
		},
		// with custom max-lease-ttl
		{
			`etcd {
			endpoint http://localhost:2379
			max-lease-ttl 1h
		}
			`, false, "skydns", []string{"http://localhost:2379"}, "", "", "",
		},
		// with both custom min-lease-ttl and max-lease-ttl
		{
			`etcd {
			endpoint http://localhost:2379
			min-lease-ttl 120
			max-lease-ttl 7200
		}
			`, false, "skydns", []string{"http://localhost:2379"}, "", "", "",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		etcd, err := etcdParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
				continue
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err.Error(), test.input)
				continue
			}
		}

		if !test.shouldErr && etcd.PathPrefix != test.expectedPath {
			t.Errorf("Etcd not correctly set for input %s. Expected: %s, actual: %s", test.input, test.expectedPath, etcd.PathPrefix)
		}
		if !test.shouldErr {
			if len(etcd.endpoints) != len(test.expectedEndpoint) {
				t.Errorf("Etcd not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.expectedEndpoint, etcd.endpoints)
			}
			for i, endpoint := range etcd.endpoints {
				if endpoint != test.expectedEndpoint[i] {
					t.Errorf("Etcd not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.expectedEndpoint, etcd.endpoints)
				}
			}
		}

		if !test.shouldErr {
			if test.username != "" {
				if etcd.Client.Username != test.username {
					t.Errorf("Etcd username not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.username, etcd.Client.Username)
				}
			}
			if test.password != "" {
				if etcd.Client.Password != test.password {
					t.Errorf("Etcd password not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.password, etcd.Client.Password)
				}
			}

			// Check TTL configuration for specific test cases
			if strings.Contains(test.input, "min-lease-ttl 60") {
				if etcd.MinLeaseTTL != 60 {
					t.Errorf("MinLeaseTTL not set correctly for input %s. Expected: 60, actual: %d", test.input, etcd.MinLeaseTTL)
				}
			}
			if strings.Contains(test.input, "max-lease-ttl 1h") {
				if etcd.MaxLeaseTTL != 3600 {
					t.Errorf("MaxLeaseTTL not set correctly for input %s. Expected: 3600, actual: %d", test.input, etcd.MaxLeaseTTL)
				}
			}
			if strings.Contains(test.input, "min-lease-ttl 120") && strings.Contains(test.input, "max-lease-ttl 7200") {
				if etcd.MinLeaseTTL != 120 {
					t.Errorf("MinLeaseTTL not set correctly for input %s. Expected: 120, actual: %d", test.input, etcd.MinLeaseTTL)
				}
				if etcd.MaxLeaseTTL != 7200 {
					t.Errorf("MaxLeaseTTL not set correctly for input %s. Expected: 7200, actual: %d", test.input, etcd.MaxLeaseTTL)
				}
			}
		}
	}
}

func TestSetupNoApexFallback(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		wantErr  bool
	}{
		{name: "default", input: `etcd`, expected: false},
		{name: "disabled", input: "etcd {\nno_apex_fallback\n}", expected: true},
		{name: "reject arguments", input: "etcd {\nno_apex_fallback unexpected\n}", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			etcd, err := etcdParse(c)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if etcd.NoApexFallback != tc.expected {
				t.Fatalf("NoApexFallback = %t, want %t", etcd.NoApexFallback, tc.expected)
			}
		})
	}
}

func TestParseTTL(t *testing.T) {
	tests := []struct {
		input    string
		expected uint32
		hasError bool
		desc     string
	}{
		// Plain numbers (assumed to be seconds)
		{"30", 30, false, "plain number should be treated as seconds"},
		{"300", 300, false, "plain number should be treated as seconds"},

		// Explicit seconds
		{"30s", 30, false, "explicit seconds"},
		{"90s", 90, false, "explicit seconds"},

		// Minutes
		{"5m", 300, false, "5 minutes"},
		{"1m", 60, false, "1 minute"},

		// Hours
		{"1h", 3600, false, "1 hour"},
		{"2h", 7200, false, "2 hours"},

		// Complex durations (Go's ParseDuration supports this)
		{"2h30m", 9000, false, "2 hours 30 minutes"},
		{"1h30m45s", 5445, false, "1 hour 30 minutes 45 seconds"},

		// Edge cases
		{"0", 0, false, "zero should be allowed"},
		{"0s", 0, false, "zero seconds should be allowed"},
		{"", 0, false, "empty string should return 0"},

		// Error cases
		{"-30s", 0, true, "negative duration should error"},
		{"abc", 0, true, "invalid format should error"},
		{"1y", 0, true, "unsupported unit should error"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := parseTTL(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("parseTTL(%q) expected error but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseTTL(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseTTL(%q) = %d, expected %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}
