package dnstap

import (
	"os"
	"testing"

	"github.com/coredns/caddy"
)

func TestConfig(t *testing.T) {
	hostname, _ := os.Hostname()
	tests := []struct {
		in       string
		endpoint string
		full     bool
		proto    string
		fail     bool
		identity []byte
		version  []byte
	}{
		{"dnstap dnstap.sock full", "dnstap.sock", true, "unix", false, []byte(hostname), []byte("-")},
		{"dnstap unix://dnstap.sock", "dnstap.sock", false, "unix", false, []byte(hostname), []byte("-")},
		{"dnstap tcp://127.0.0.1:6000", "127.0.0.1:6000", false, "tcp", false, []byte(hostname), []byte("-")},
		{"dnstap tcp://[::1]:6000", "[::1]:6000", false, "tcp", false, []byte(hostname), []byte("-")},
		{"dnstap tcp://example.com:6000", "example.com:6000", false, "tcp", false, []byte(hostname), []byte("-")},
		{"dnstap", "fail", false, "tcp", true, []byte(hostname), []byte("-")},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\n}\n", "dnstap.sock", true, "unix", false, []byte("NAME"), []byte("VER")},
		{"dnstap dnstap.sock {\nidentity NAME\nversion VER\n}\n", "dnstap.sock", false, "unix", false, []byte("NAME"), []byte("VER")},
		{"dnstap {\nidentity NAME\nversion VER\n}\n", "fail", false, "tcp", true, []byte("NAME"), []byte("VER")},
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
		if x := string(tap.Identity); x != string(tc.identity) {
			t.Errorf("Test %d: expected identity %s, got %s", i, tc.identity, x)
		}
		if x := string(tap.Version); x != string(tc.version) {
			t.Errorf("Test %d: expected version %s, got %s", i, tc.version, x)
		}
	}
}
