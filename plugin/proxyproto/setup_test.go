package proxyproto

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input                      string
		shouldErr                  bool
		expectedRoot               string // expected root, set to the controller. Empty for negative cases.
		expectedErrContent         string // substring from the expected error. Empty for positive cases.
		config                     bool
		sessionTrackingTTL         time.Duration
		sessionTrackingMaxSessions int
	}{
		// positive
		{"proxyproto", false, "", "", true, 0, 0},
		{"proxyproto {\nallow 127.0.0.1/8 ::1/128\n}", false, "", "", true, 0, 0},
		{"proxyproto {\nallow 127.0.0.1/8 ::1/128\ndefault ignore\n}", false, "", "", true, 0, 0},
		// Allow without any IPs is also valid
		{"proxyproto {\nallow\n}", false, "", "", true, 0, 0},
		// udp_session_tracking with TTL only (max sessions gonna use package default)
		{"proxyproto {\nudp_session_tracking 28s\n}", false, "", "", true, 28 * time.Second, 0},
		{"proxyproto {\nallow 10.0.0.0/8\nudp_session_tracking 1m\n}", false, "", "", true, time.Minute, 0},
		// udp_session_tracking with explicit max sessions
		{"proxyproto {\nudp_session_tracking 28s 20000\n}", false, "", "", true, 28 * time.Second, 20000},
		{"proxyproto {\nallow 10.0.0.0/8\nudp_session_tracking 1m 500\n}", false, "", "", true, time.Minute, 500},
		// negative
		{"proxyproto {\nunknown\n}", true, "", "unknown option", false, 0, 0},
		{"proxyproto extra_arg", true, "", "Wrong argument", false, 0, 0},
		{"proxyproto {\nallow invalid_ip\n}", true, "", "invalid CIDR address", false, 0, 0},
		{"proxyproto {\nallow 127.0.0.1/8\ndefault invalid_policy\n}", true, "", "Wrong argument", false, 0, 0},
		// udp_session_tracking: missing TTL
		{"proxyproto {\nudp_session_tracking\n}", true, "", "Wrong argument", false, 0, 0},
		// udp_session_tracking: too many arguments
		{"proxyproto {\nudp_session_tracking 28s 100 extra\n}", true, "", "Wrong argument", false, 0, 0},
		// udp_session_tracking: bad TTL
		{"proxyproto {\nudp_session_tracking notaduration\n}", true, "", "invalid duration", false, 0, 0},
		// udp_session_tracking: bad max sessions
		{"proxyproto {\nudp_session_tracking 28s notanumber\n}", true, "", "invalid max sessions", false, 0, 0},
		{"proxyproto {\nudp_session_tracking 28s 0\n}", true, "", "invalid max sessions", false, 0, 0},
		{"proxyproto {\nudp_session_tracking 28s -1\n}", true, "", "invalid max sessions", false, 0, 0},
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

		if cfg.ProxyProtoUDPSessionTrackingTTL != test.sessionTrackingTTL {
			t.Errorf("Test %d: Expected ProxyProtoUDPSessionTrackingTTL %v, got %v for input %s",
				i, test.sessionTrackingTTL, cfg.ProxyProtoUDPSessionTrackingTTL, test.input)
		}

		if cfg.ProxyProtoUDPSessionTrackingMaxSessions != test.sessionTrackingMaxSessions {
			t.Errorf("Test %d: Expected ProxyProtoUDPSessionTrackingMaxSessions %d, got %d for input %s",
				i, test.sessionTrackingMaxSessions, cfg.ProxyProtoUDPSessionTrackingMaxSessions, test.input)
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
