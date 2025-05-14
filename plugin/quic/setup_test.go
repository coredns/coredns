package quic

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestQuicSetup(t *testing.T) {
	tests := []struct {
		input                  string
		shouldErr              bool
		expectedMaxStreams     *int
		expectedWorkerPoolSize *int
		expectedErrContent     string
	}{
		// Valid configurations
		{
			input:                  `quic`,
			shouldErr:              false,
			expectedMaxStreams:     nil,
			expectedWorkerPoolSize: nil,
		},
		{
			input: `quic {
			}`,
			shouldErr:              false,
			expectedMaxStreams:     nil,
			expectedWorkerPoolSize: nil,
		},
		{
			input: `quic {
				max_streams 100
			}`,
			shouldErr:              false,
			expectedMaxStreams:     pint(100),
			expectedWorkerPoolSize: nil,
		},
		{
			input: `quic {
				worker_pool_size 1000
			}`,
			shouldErr:              false,
			expectedMaxStreams:     nil,
			expectedWorkerPoolSize: pint(1000),
		},
		{
			input: `quic {
				max_streams 100
				worker_pool_size 1000
			}`,
			shouldErr:              false,
			expectedMaxStreams:     pint(100),
			expectedWorkerPoolSize: pint(1000),
		},
		{
			input: `quic {
				# Comment
			}`,
			shouldErr:              false,
			expectedMaxStreams:     nil,
			expectedWorkerPoolSize: nil,
		},
		// Invalid configurations
		{
			input:              `quic arg`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `quic {
				max_streams
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `quic {
				max_streams abc
			}`,
			shouldErr:          true,
			expectedErrContent: "invalid max_streams value",
		},
		{
			input: `quic {
				max_streams 0
			}`,
			shouldErr:          true,
			expectedErrContent: "positive integer",
		},
		{
			input: `quic {
				max_streams -10
			}`,
			shouldErr:          true,
			expectedErrContent: "positive integer",
		},
		{
			input: `quic {
				worker_pool_size
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `quic {
				worker_pool_size abc
			}`,
			shouldErr:          true,
			expectedErrContent: "invalid worker_pool_size value",
		},
		{
			input: `quic {
				worker_pool_size 0
			}`,
			shouldErr:          true,
			expectedErrContent: "positive integer",
		},
		{
			input: `quic {
				worker_pool_size -10
			}`,
			shouldErr:          true,
			expectedErrContent: "positive integer",
		},
		{
			input: `quic {
				max_streams 100
				max_streams 200
			}`,
			shouldErr:          true,
			expectedErrContent: "already defined",
			expectedMaxStreams: pint(100),
		},
		{
			input: `quic {
				worker_pool_size 1000
				worker_pool_size 2000
			}`,
			shouldErr:              true,
			expectedErrContent:     "already defined",
			expectedWorkerPoolSize: pint(1000),
		},
		{
			input: `quic {
				unknown_directive
			}`,
			shouldErr:          true,
			expectedErrContent: "unknown property",
		},
		{
			input: `quic {
				max_streams 100 200
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `quic {
				worker_pool_size 1000 2000
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d (%s): Expected error but found none", i, test.input)
			continue
		}
		if !test.shouldErr && err != nil {
			t.Errorf("Test %d (%s): Expected no error but found: %v", i, test.input, err)
			continue
		}

		if test.shouldErr && !strings.Contains(err.Error(), test.expectedErrContent) {
			t.Errorf("Test %d (%s): Expected error containing '%s', but got: %v",
				i, test.input, test.expectedErrContent, err)
			continue
		}

		if !test.shouldErr || (test.shouldErr && strings.Contains(test.expectedErrContent, "already defined")) {
			config := dnsserver.GetConfig(c)
			assertMaxStreamsValue(t, i, test.input, config.MaxQUICStreams, test.expectedMaxStreams)
			assertWorkerPoolSizeValue(t, i, test.input, config.MaxQUICWorkerPoolSize, test.expectedWorkerPoolSize)
		}
	}
}

// assertMaxStreamsValue compares the actual MaxQUICStreams value with the expected one
func assertMaxStreamsValue(t *testing.T, testIndex int, testInput string, actual, expected *int) {
	if actual == nil && expected == nil {
		return
	}

	if (actual == nil) != (expected == nil) {
		t.Errorf("Test %d (%s): Expected MaxQUICStreams to be %v, but got %v",
			testIndex, testInput, formatNilableInt(expected), formatNilableInt(actual))
		return
	}

	if *actual != *expected {
		t.Errorf("Test %d (%s): Expected MaxQUICStreams to be %d, but got %d",
			testIndex, testInput, *expected, *actual)
	}
}

// assertWorkerPoolSizeValue compares the actual MaxQUICWorkerPoolSize value with the expected one
func assertWorkerPoolSizeValue(t *testing.T, testIndex int, testInput string, actual, expected *int) {
	if actual == nil && expected == nil {
		return
	}

	if (actual == nil) != (expected == nil) {
		t.Errorf("Test %d (%s): Expected MaxQUICWorkerPoolSize to be %v, but got %v",
			testIndex, testInput, formatNilableInt(expected), formatNilableInt(actual))
		return
	}

	if *actual != *expected {
		t.Errorf("Test %d (%s): Expected MaxQUICWorkerPoolSize to be %d, but got %d",
			testIndex, testInput, *expected, *actual)
	}
}

func formatNilableInt(v *int) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *v)
}

func pint(i int) *int {
	return &i
}
