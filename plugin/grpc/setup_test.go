package grpc

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/fall"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input           string
		shouldErr       bool
		expectedFrom    string
		expectedIgnored []string
		expectedErr     string
	}{
		// positive
		{"grpc . 127.0.0.1", false, ".", nil, ""},
		{"grpc . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", nil, ""},
		{"grpc . 127.0.0.1", false, ".", nil, ""},
		{"grpc . 127.0.0.1:53", false, ".", nil, ""},
		{"grpc . 127.0.0.1:8080", false, ".", nil, ""},
		{"grpc . [::1]:53", false, ".", nil, ""},
		{"grpc . [2003::1]:53", false, ".", nil, ""},
		{"grpc . unix:///var/run/g.sock", false, ".", nil, ""},
		// negative
		{"grpc . a27.0.0.1", true, "", nil, "not an IP"},
		{"grpc . 127.0.0.1 {\nblaatl\n}\n", true, "", nil, "unknown property"},
		{`grpc . ::1
		grpc com ::2`, true, "", nil, "plugin"},
		{"grpc xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx 127.0.0.1", true, "", nil, "unable to normalize 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("grpc", test.input)
		g, err := parseGRPC(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && g.from != test.expectedFrom {
			t.Errorf("Test %d: expected: %s, got: %s", i, test.expectedFrom, g.from)
		}
		if !test.shouldErr && test.expectedIgnored != nil {
			if !reflect.DeepEqual(g.ignored, test.expectedIgnored) {
				t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, g.ignored)
			}
		}
	}
}

func TestSetupTLS(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedServerName string
		expectedErr        string
	}{
		// positive
		{`grpc . 127.0.0.1 {
tls_servername dns
}`, false, "dns", ""},
		{`grpc . 127.0.0.1 {
tls
}`, false, "", ""},
		{`grpc . 127.0.0.1`, false, "", ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		g, err := parseGRPC(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && test.expectedServerName != "" && g.tlsConfig != nil && test.expectedServerName != g.tlsConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, g.tlsConfig.ServerName)
		}
	}
}

func TestSetupResolvconf(t *testing.T) {
	const resolv = "resolv.conf"
	if err := os.WriteFile(resolv,
		[]byte(`nameserver 10.10.255.252
nameserver 10.10.255.253`), 0666); err != nil {
		t.Fatalf("Failed to write resolv.conf file: %s", err)
	}
	defer os.Remove(resolv)

	tests := []struct {
		input         string
		shouldErr     bool
		expectedErr   string
		expectedNames []string
	}{
		// pass
		{`grpc . ` + resolv, false, "", []string{"10.10.255.252:53", "10.10.255.253:53"}},
	}

	for i, test := range tests {
		c := caddy.NewTestController("grpc", test.input)
		f, err := parseGRPC(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
			continue
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr {
			for j, n := range test.expectedNames {
				addr := f.proxies[j].addr
				if n != addr {
					t.Errorf("Test %d, expected %q, got %q", j, n, addr)
				}
			}
		}
	}
}

func TestSetupFallthrough(t *testing.T) {
	tests := []struct {
		input               string
		shouldErr           bool
		expectedFallthrough fall.F
		expectedErr         string
	}{
		// positive cases
		{`grpc . 127.0.0.1 {
	fallthrough
}`, false, fall.Root, ""},
		{`grpc . 127.0.0.1 {
	fallthrough example.org
}`, false, fall.F{Zones: []string{"example.org."}}, ""},
		{`grpc . 127.0.0.1 {
	fallthrough example.org example.com
}`, false, fall.F{Zones: []string{"example.org.", "example.com."}}, ""},
		{`grpc . 127.0.0.1`, false, fall.Zero, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		g, err := parseGRPC(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found none for input %s", i, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && !g.Fall.Equal(test.expectedFallthrough) {
			t.Errorf("Test %d: expected fallthrough %+v, got %+v", i, test.expectedFallthrough, g.Fall)
		}
	}
}

func TestSetupPooling(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		errStr    string
	}{
		// valid
		{"grpc . 127.0.0.1 {\npool_size 2\n}", false, ""},
		{"grpc . 127.0.0.1 {\npool_size 100\n}", false, ""},
		{"grpc . 127.0.0.1 {\npool_size 2\nexpire 30s\nhealth_check 1s\nmax_fails 3\n}", false, ""},
		// invalid
		{"grpc . 127.0.0.1 {\npool_size 0\n}", true, "pool_size must be at least 1"},
		{"grpc . 127.0.0.1 {\npool_size 101\n}", true, "pool_size cannot exceed 100"},
		{"grpc . 127.0.0.1 {\npool_size -1\n}", true, ""},
		{"grpc . 127.0.0.1 {\nexpire -1s\n}", true, "expire can't be negative"},
		{"grpc . 127.0.0.1 {\nhealth_check -1s\n}", true, "health_check can't be negative"},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		g, err := parseGRPC(c)
		if tc.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but got none for input: %s", i, tc.input)
		}
		if !tc.shouldErr && err != nil {
			t.Errorf("Test %d: expected no error but got: %v for input: %s", i, err, tc.input)
		}
		if tc.shouldErr && err != nil && tc.errStr != "" && !strings.Contains(err.Error(), tc.errStr) {
			t.Errorf("Test %d: expected error containing %q, got: %v", i, tc.errStr, err)
		}
		// cleanup pooled proxies to avoid goroutine leaks
		if err == nil && g != nil {
			for _, p := range g.proxies {
				p.Stop()
			}
		}
	}
}

func TestSetupPooled_ProxyHasTransport(t *testing.T) {
	// pool_size > 1 → proxy must have a transport
	c := caddy.NewTestController("dns", "grpc . 127.0.0.1 {\npool_size 3\n}")
	g, err := parseGRPC(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.proxies) == 0 {
		t.Fatal("expected at least one proxy")
	}
	if g.proxies[0].transport == nil {
		t.Error("expected proxy to have a transport when pool_size > 1")
	}
	if g.proxies[0].client != nil {
		t.Error("expected proxy to NOT have a direct client when pool_size > 1")
	}
	// cleanup
	for _, p := range g.proxies {
		p.Stop()
	}
}

func TestSetupSingle_ProxyHasClient(t *testing.T) {
	// pool_size=1 (default) → proxy must have a client, no transport
	c := caddy.NewTestController("dns", "grpc . 127.0.0.1")
	g, err := parseGRPC(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.proxies) == 0 {
		t.Fatal("expected at least one proxy")
	}
	if g.proxies[0].client == nil {
		t.Error("expected proxy to have a direct client when pool_size=1")
	}
	if g.proxies[0].transport != nil {
		t.Error("expected proxy to NOT have a transport when pool_size=1")
	}
}
