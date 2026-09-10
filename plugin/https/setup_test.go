package https

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	// The uint32 upper-bound boundary is architecture-dependent. On 64-bit int,
	// 4294967295 is accepted and 4294967296 hits the "must not exceed" guard. On
	// 32-bit int, both overflow and Atoi rejects them as "invalid max_streams"
	// first. Derive the value via Atoi (never a constant literal) so the source
	// stays portable to 32-bit targets.
	maxUint32Err, maxUint32ErrContent := false, ""
	var maxUint32Streams *int
	overMaxUint32ErrContent := "must not exceed"
	if v, err := strconv.Atoi("4294967295"); err == nil {
		maxUint32Streams = &v
	} else {
		maxUint32Err, maxUint32ErrContent = true, "invalid max_streams value"
		overMaxUint32ErrContent = "invalid max_streams value"
	}

	tests := []struct {
		input                  string
		shouldErr              bool
		expectedErrContent     string
		expectedMaxConnections *int
		expectedMaxStreams     *int
	}{
		// Valid configurations
		{
			input:     `https`,
			shouldErr: false,
		},
		{
			input: `https {
			}`,
			shouldErr: false,
		},
		{
			input: `https {
				max_connections 200
			}`,
			shouldErr:              false,
			expectedMaxConnections: new(200),
		},
		{
			input: `https {
				max_streams 100
			}`,
			shouldErr:          false,
			expectedMaxStreams: new(100),
		},
		{
			input: `https {
				max_connections 200
				max_streams 100
			}`,
			shouldErr:              false,
			expectedMaxConnections: new(200),
			expectedMaxStreams:     new(100),
		},
		// Zero values (unbounded)
		{
			input: `https {
				max_connections 0
			}`,
			shouldErr:              false,
			expectedMaxConnections: new(0),
		},
		// Error cases
		{
			input: `https {
				max_connections
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `https {
				max_connections abc
			}`,
			shouldErr:          true,
			expectedErrContent: "invalid max_connections value",
		},
		{
			input: `https {
				max_connections -1
			}`,
			shouldErr:          true,
			expectedErrContent: "must be a non-negative integer",
		},
		{
			input: `https {
				max_connections 100
				max_connections 200
			}`,
			shouldErr:          true,
			expectedErrContent: "already defined",
		},
		{
			input: `https {
				max_streams
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `https {
				max_streams abc
			}`,
			shouldErr:          true,
			expectedErrContent: "invalid max_streams value",
		},
		{
			input: `https {
				max_streams 0
			}`,
			shouldErr:          false,
			expectedMaxStreams: new(0),
		},
		{
			input: `https {
				max_streams -1
			}`,
			shouldErr:          true,
			expectedErrContent: "must be a non-negative integer",
		},
		{
			input: `https {
				max_streams 4294967295
			}`,
			shouldErr:          maxUint32Err,
			expectedErrContent: maxUint32ErrContent,
			expectedMaxStreams: maxUint32Streams,
		},
		{
			input: `https {
				max_streams 4294967296
			}`,
			shouldErr:          true,
			expectedErrContent: overMaxUint32ErrContent,
		},
		{
			input: `https {
				max_streams 100
				max_streams 200
			}`,
			shouldErr:          true,
			expectedErrContent: "already defined",
		},
		{
			input: `https {
				unknown_option 123
			}`,
			shouldErr:          true,
			expectedErrContent: "unknown property",
		},
		{
			input:              `https extra_arg`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d (%s): Expected error but got none", i, test.input)
			continue
		}

		if !test.shouldErr && err != nil {
			t.Errorf("Test %d (%s): Expected no error but got: %v", i, test.input, err)
			continue
		}

		if test.shouldErr && test.expectedErrContent != "" {
			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d (%s): Expected error containing '%s' but got: %v",
					i, test.input, test.expectedErrContent, err)
			}
			continue
		}

		if !test.shouldErr {
			config := dnsserver.GetConfig(c)
			assertIntPtrValue(t, i, test.input, "MaxHTTPSConnections", config.MaxHTTPSConnections, test.expectedMaxConnections)
			assertIntPtrValue(t, i, test.input, "MaxHTTPSStreams", config.MaxHTTPSStreams, test.expectedMaxStreams)
		}
	}
}

//go:fix inline
func intPtr(v int) *int {
	return new(v)
}

func assertIntPtrValue(t *testing.T, testIndex int, testInput, fieldName string, actual, expected *int) {
	t.Helper()
	if actual == nil && expected == nil {
		return
	}

	if (actual == nil) != (expected == nil) {
		t.Errorf("Test %d (%s): Expected %s to be %v, but got %v",
			testIndex, testInput, fieldName, formatNilableInt(expected), formatNilableInt(actual))
		return
	}

	if *actual != *expected {
		t.Errorf("Test %d (%s): Expected %s to be %d, but got %d",
			testIndex, testInput, fieldName, *expected, *actual)
	}
}

func formatNilableInt(v *int) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *v)
}
