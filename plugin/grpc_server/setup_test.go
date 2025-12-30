package grpc_server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input                  string
		shouldErr              bool
		expectedErrContent     string
		expectedMaxStreams     *int
		expectedMaxConnections *int
	}{
		// Valid configurations
		{
			input:     `grpc_server`,
			shouldErr: false,
		},
		{
			input: `grpc_server {
			}`,
			shouldErr: false,
		},
		{
			input: `grpc_server {
				max_streams 100
			}`,
			shouldErr:          false,
			expectedMaxStreams: intPtr(100),
		},
		{
			input: `grpc_server {
				max_connections 200
			}`,
			shouldErr:              false,
			expectedMaxConnections: intPtr(200),
		},
		{
			input: `grpc_server {
				max_streams 50
				max_connections 100
			}`,
			shouldErr:              false,
			expectedMaxStreams:     intPtr(50),
			expectedMaxConnections: intPtr(100),
		},
		// Zero values (unbounded)
		{
			input: `grpc_server {
				max_streams 0
			}`,
			shouldErr:          false,
			expectedMaxStreams: intPtr(0),
		},
		{
			input: `grpc_server {
				max_connections 0
			}`,
			shouldErr:              false,
			expectedMaxConnections: intPtr(0),
		},
		// Error cases
		{
			input: `grpc_server {
				max_streams
			}`,
			shouldErr:          true,
			expectedErrContent: "Wrong argument count",
		},
		{
			input: `grpc_server {
				max_streams abc
			}`,
			shouldErr:          true,
			expectedErrContent: "invalid max_streams value",
		},
		{
			input: `grpc_server {
				max_streams -1
			}`,
			shouldErr:          true,
			expectedErrContent: "must be a non-negative integer",
		},
		{
			input: `grpc_server {
				max_streams 100
				max_streams 200
			}`,
			shouldErr:          true,
			expectedErrContent: "already defined",
		},
		{
			input: `grpc_server {
				unknown_option 123
			}`,
			shouldErr:          true,
			expectedErrContent: "unknown property",
		},
		{
			input:              `grpc_server extra_arg`,
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
			assertIntPtrValue(t, i, test.input, "MaxGRPCStreams", config.MaxGRPCStreams, test.expectedMaxStreams)
			assertIntPtrValue(t, i, test.input, "MaxGRPCConnections", config.MaxGRPCConnections, test.expectedMaxConnections)
		}
	}
}

func intPtr(v int) *int {
	return &v
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
