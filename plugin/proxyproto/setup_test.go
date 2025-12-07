package proxyproto

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedRoot       string // expected root, set to the controller. Empty for negative cases.
		expectedErrContent string // substring from the expected error. Empty for positive cases.
		config             bool
	}{
		// positive
		{"proxyproto", false, "", "", true},
		{"proxyproto {\nallow 127.0.0.1/8 ::1/128\n}", false, "", "", true},
		{"proxyproto {\nallow 127.0.0.1/8 ::1/128\ndefault ignore\n}", false, "", "", true},
		// Allow without any IPs is also valid
		{"proxyproto {\nallow\n}", false, "", "", true},
		// negative
		{"proxyproto {\nunknown\n}", true, "", "unknown option", false},
		{"proxyproto extra_arg", true, "", "Wrong argument", false},
		{"proxyproto {\nallow invalid_ip\n}", true, "", "invalid CIDR address", false},
		{"proxyproto {\nallow 127.0.0.1/8\ndefault invalid_policy\n}", true, "", "Wrong argument", false},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)
		cfg := dnsserver.GetConfig(c)

		if test.config && cfg.ProxyProtoConnPolicy == nil {
			t.Errorf("Test %d: Expected ProxyProtoConnPolicy to be configured for input %s", i, test.input)
		}
		if !test.config && cfg.ProxyProtoConnPolicy != nil {
			t.Errorf("Test %d: Expected ProxyProtoConnPolicy to NOT be configured for input %s", i, test.input)
		}

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err, test.input)
			}
		}
	}
}
