package shed

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		{"shed", false},
		{"shed extra", true},
		{"shed {\n depth 10\n}", true},
		{"shed\nshed", true},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		err := setup(c)
		if tc.shouldErr && err == nil {
			t.Errorf("Test %d: expected error for input %q", i, tc.input)
		}
		if !tc.shouldErr && err != nil {
			t.Errorf("Test %d: unexpected error for input %q: %s", i, tc.input, err)
		}
	}
}

func TestSetupRejectsNonDNSTransport(t *testing.T) {
	for _, key := range []string{"tls://.:853", "grpc://.:443", "https://.:443", "quic://.:853"} {
		c := caddy.NewTestController("dns", "shed")
		c.ServerBlockKeys = []string{key}
		err := setup(c)
		if err == nil {
			t.Errorf("expected error for server block key %q", key)
			continue
		}
		if !strings.Contains(err.Error(), "plain DNS") {
			t.Errorf("error for %q = %q, want it to mention plain DNS", key, err)
		}
	}
}

func TestSetupAcceptsPlainDNSKeys(t *testing.T) {
	c := caddy.NewTestController("dns", "shed")
	c.ServerBlockKeys = []string{"example.org.:53", "dns://.:53"}
	if err := setup(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}
