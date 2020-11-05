package dnstap

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestConfig(t *testing.T) {
	tests := []struct {
		in       string
		endpoint string
		full     bool
		proto    string
		fail     bool
	}{
		{"dnstap dnstap.sock full", "dnstap.sock", true, "unix", false},
		{"dnstap unix://dnstap.sock", "dnstap.sock", false, "unix", false},
		{"dnstap tcp://127.0.0.1:6000", "127.0.0.1:6000", false, "tcp", false},
		{"dnstap", "fail", false, "tcp", true},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.in)
		tap, err := parseConfig(c)
		if tc.fail && err == nil {
			t.Fatalf("Test %d: expected test to fail: %s: %s", i, tc.in, err)
		}
		if tc.fail {
			continue
		}

		if err != nil {
			t.Fatalf("Test %d: expected no error, got %s", i, err)
		}
		if x := tap.io.(*dio).endpoint; x != tc.endpoint {
			t.Errorf("Test %d: expected endpoint %s, got %s", i, tc.endpoint, x)
		}
		if x := tap.io.(*dio).proto; x != tc.proto {
			t.Errorf("Test %d: expected proto %s, got %s", i, tc.proto, x)
		}
		if x := tap.IncludeRawMessage; x != tc.full {
			t.Errorf("Test %d: expected IncludeRawMessage %t, got %t", i, tc.full, x)
		}
	}
}
