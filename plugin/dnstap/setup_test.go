package dnstap

import (
	"os"
	"testing"

	"github.com/coredns/caddy"
)

type results struct{
endpoint string
full     bool
proto    string
identity []byte
version  []byte
}

func TestConfig(t *testing.T) {
	hostname, _ := os.Hostname()
	tests := []struct {
		in       string
		fail     bool
		expect []results
	}{
		{"dnstap dnstap.sock full", false, []results{{"dnstap.sock", true, "unix",  []byte(hostname), []byte("-")}}},
		{"dnstap unix://dnstap.sock", false, []results{{"dnstap.sock", false, "unix", []byte(hostname), []byte("-")}}},
		{"dnstap tcp://127.0.0.1:6000",false, []results{{"127.0.0.1:6000", false, "tcp",  []byte(hostname), []byte("-")}}},
		{"dnstap tcp://[::1]:6000", false,[]results{{"[::1]:6000", false, "tcp",  []byte(hostname), []byte("-")}}},
		{"dnstap tcp://example.com:6000", false, []results{{"example.com:6000", false, "tcp",  []byte(hostname), []byte("-")}}},
		{"dnstap", true, []results{{"fail", false, "tcp",  []byte(hostname), []byte("-")}}},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\n}\n", false,[]results{{"dnstap.sock", true, "unix",  []byte("NAME"), []byte("VER")}}},
		{"dnstap dnstap.sock {\nidentity NAME\nversion VER\n}\n", false,[]results{{"dnstap.sock", false, "unix",  []byte("NAME"), []byte("VER")}}},
		{"dnstap {\nidentity NAME\nversion VER\n}\n", true,[]results{{"fail", false, "tcp",  []byte("NAME"), []byte("VER")}}},
		{`dnstap dnstap.sock full {
                identity NAME
                version VER
              }
              dnstap tcp://127.0.0.1:6000 {
                identity NAME2
                version VER2
              }`, false,[]results{
			{"dnstap.sock", true, "unix",  []byte("NAME"), []byte("VER")},
			{"127.0.0.1:6000", false, "tcp",  []byte("NAME2"), []byte("VER2")},
		}},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.in)
		taps, err := parseConfig(c)
		if tc.fail && err == nil {
			t.Fatalf("Test %d: expected test to fail: %s: %s", i, tc.in, err)
		}
		if tc.fail {
			continue
		}

		if err != nil {
			t.Fatalf("Test %d: expected no error, got %s", i, err)
		}
		for i, tap := range taps {
			if x := tap.io.(*dio).endpoint; x != tc.expect[i].endpoint {
				t.Errorf("Test %d: expected endpoint %s, got %s", i, tc.expect[i].endpoint, x)
			}
			if x := tap.io.(*dio).proto; x != tc.expect[i].proto {
				t.Errorf("Test %d: expected proto %s, got %s", i, tc.expect[i].proto, x)
			}
			if x := tap.IncludeRawMessage; x != tc.expect[i].full {
				t.Errorf("Test %d: expected IncludeRawMessage %t, got %t", i, tc.expect[i].full, x)
			}
			if x := string(tap.Identity); x != string(tc.expect[i].identity) {
				t.Errorf("Test %d: expected identity %s, got %s", i, tc.expect[i].identity, x)
			}
			if x := string(tap.Version); x != string(tc.expect[i].version) {
				t.Errorf("Test %d: expected version %s, got %s", i, tc.expect[i].version, x)
			}
		}
	}
}
