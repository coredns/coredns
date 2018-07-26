package forward

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input           string
		shouldErr       bool
		expectedFrom    string
		expectedIgnored []string
		expectedFails   uint32
		expectedOpts    options
		expectedProxies int
		expectedErr     string
	}{
		// positive
		{"forward . 127.0.0.1", false, ".", nil, 2, options{}, 1, ""},
		{"forward . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", nil, 2, options{}, 1, ""},
		{"forward . 127.0.0.1 {\nmax_fails 3\n}\n", false, ".", nil, 3, options{}, 1, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\n}\n", false, ".", nil, 2, options{forceTCP: true}, 1, ""},
		{"forward . 127.0.0.1 {\nprefer_udp\n}\n", false, ".", nil, 2, options{preferUDP: true}, 1, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\nprefer_udp\n}\n", false, ".", nil, 2, options{preferUDP: true, forceTCP: true}, 1, ""},
		{"forward . 127.0.0.1:53", false, ".", nil, 2, options{}, 1, ""},
		{"forward . 127.0.0.1:8080", false, ".", nil, 2, options{}, 1, ""},
		{"forward . [::1]:53", false, ".", nil, 2, options{}, 1, ""},
		{"forward . [2003::1]:53", false, ".", nil, 2, options{}, 1, ""},
		{"forward . fixtures/resolv.conf", false, ".", nil, 2, options{}, 2, ""},
		// negative
		{"forward . a27.0.0.1", true, "", nil, 0, options{}, 0, "not an IP"},
		{"forward . 127.0.0.1 {\nblaatl\n}\n", true, "", nil, 0, options{}, 0, "unknown property"},
		{`forward . ::1
		forward com ::2`, true, "", nil, 0, options{}, 0, "plugin"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

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

			continue
		}

		if f.from != test.expectedFrom {
			t.Errorf("Test %d: expected: %s, got: %s", i, test.expectedFrom, f.from)
		}
		if test.expectedIgnored != nil {
			if !reflect.DeepEqual(f.ignored, test.expectedIgnored) {
				t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, f.ignored)
			}
		}
		if f.maxfails != test.expectedFails {
			t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedFails, f.maxfails)
		}
		if f.opts != test.expectedOpts {
			t.Errorf("Test %d: expected: %v, got: %v", i, test.expectedOpts, f.opts)
		}

		if len(f.proxies) != test.expectedProxies {
			t.Errorf("Test %d: expected %d proxies, got %d", i, test.expectedProxies, len(f.proxies))
		}

		for n, p := range f.proxies {
			if p.health == nil {
				t.Errorf("Test %d: Proxy %d (%#v): want HealthCheck function, got nil", i, n, p)
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
		{`forward . tls://127.0.0.1 {
				tls_servername dns
			}`, false, "dns", ""},
		{`forward . 127.0.0.1 {
				tls_servername dns
			}`, false, "", ""},
		{`forward . 127.0.0.1 {
				tls
			}`, false, "", ""},
		{`forward . tls://127.0.0.1`, false, "", ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

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

			continue
		}

		if test.expectedServerName != "" && test.expectedServerName != f.tlsConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.tlsConfig.ServerName)
		}

		if test.expectedServerName != "" && test.expectedServerName != f.proxies[0].health.(*dnsHc).c.TLSConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.proxies[0].health.(*dnsHc).c.TLSConfig.ServerName)
		}
	}
}
