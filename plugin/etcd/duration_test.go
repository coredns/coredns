package etcd

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
)

// These tests cover the sync_interval/dial_timeout directives and the
// parseNonZeroDuration helper. They are kept out of setup_test.go (which
// carries the "etcd" build tag) because etcdParse does not need a live
// etcd server to run: the etcd client dials non-blocking, so these cases
// run as part of the default `go test ./...` CI job.
func TestSetupEtcdDurationDirectives(t *testing.T) {
	tests := []struct {
		input                string
		shouldErr            bool
		expectedErrContent   string
		expectedSyncInterval time.Duration
		expectedDialTimeout  time.Duration
	}{
		// with custom sync_interval
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval 1m
		}
			`, false, "", time.Minute, 0,
		},
		// with custom dial_timeout
		{
			`etcd {
			endpoint http://localhost:2379
			dial_timeout 5s
		}
			`, false, "", 0, 5 * time.Second,
		},
		// with both set
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval 1m
			dial_timeout 5s
		}
			`, false, "", time.Minute, 5 * time.Second,
		},
		// with plain-number sync_interval (treated as seconds)
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval 30
		}
			`, false, "", 30 * time.Second, 0,
		},
		// with invalid sync_interval
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval notaduration
		}
			`, true, "invalid sync_interval value", 0, 0,
		},
		// with zero sync_interval (rejected -- omit the directive to disable auto-sync)
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval 0
		}
			`, true, "invalid sync_interval value", 0, 0,
		},
		// with invalid dial_timeout
		{
			`etcd {
			endpoint http://localhost:2379
			dial_timeout -5s
		}
			`, true, "invalid dial_timeout value", 0, 0,
		},
		// with missing sync_interval argument
		{
			`etcd {
			endpoint http://localhost:2379
			sync_interval
		}
			`, true, "Wrong argument count", 0, 0,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		etcd, err := etcdParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found none for input %s", i, test.input)
			continue
		}
		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: expected no error but found one for input %s. Error was: %v", i, test.input, err)
			continue
		}
		if err != nil {
			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err.Error(), test.input)
			}
			continue
		}

		if etcd.syncInterval != test.expectedSyncInterval {
			t.Errorf("Test %d: syncInterval not correctly reaching client config for input %s. Expected: %v, actual: %v", i, test.input, test.expectedSyncInterval, etcd.syncInterval)
		}
		if etcd.dialTimeout != test.expectedDialTimeout {
			t.Errorf("Test %d: dialTimeout not correctly reaching client config for input %s. Expected: %v, actual: %v", i, test.input, test.expectedDialTimeout, etcd.dialTimeout)
		}
	}
}

func TestParseNonZeroDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		hasError bool
		desc     string
	}{
		{"30", 30 * time.Second, false, "plain number should be treated as seconds"},
		{"30s", 30 * time.Second, false, "explicit seconds"},
		{"1m", time.Minute, false, "1 minute"},
		{"1h", time.Hour, false, "1 hour"},
		{"2h30m", 2*time.Hour + 30*time.Minute, false, "2 hours 30 minutes"},

		{"0", 0, true, "zero should be rejected"},
		{"0s", 0, true, "zero seconds should be rejected"},
		{"-30s", 0, true, "negative duration should be rejected"},
		{"", 0, true, "empty string should error"},
		{"abc", 0, true, "invalid format should error"},
		{"1y", 0, true, "unsupported unit should error"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := parseNonZeroDuration(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("parseNonZeroDuration(%q) expected error but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseNonZeroDuration(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseNonZeroDuration(%q) = %v, expected %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}
