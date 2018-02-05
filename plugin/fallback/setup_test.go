package fallback

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mholt/caddy"
)

type setupTestCase struct {
	config        string
	expectedError string
}

func TestSetupFallback(t *testing.T) {
	testCases := []setupTestCase{
		{
			config: `fallback REFUSED . 192.168.1.1:53`,
		},
		{
			config: `fallback SERVFAIL . 192.168.1.1:53`,
		},
		{
			config: `fallback NXDOMAIN . 192.168.1.1:53`,
		},
		{
			config:        `fallback REFUSE . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config:        `fallback SRVFAIL . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config:        `fallback NODOMAIN . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config: `fallback REFUSED . 192.168.1.1:53 {
						max_fails 5
						protocol grpc
					}`,
		},
		{
			config:        `fallback REFUSED . abc`,
			expectedError: `not an IP address or file`,
		},
		{
			config: `fallback REFUSED . 192.168.1.1:53
					 fallback REFUSED . 192.168.1.2:53`,
			expectedError: `specified more than once`,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s", tc.config), func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.config)
			err := setup(c)
			if err == nil {
				if tc.expectedError != "" {
					t.Fatalf("Expected error '%s', but got no error", tc.expectedError)
				}
			} else {
				if tc.expectedError == "" {
					t.Fatalf("Expected no error, but got '%s'", err)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("Expected error '%s', but got '%s'", tc.expectedError, err)
				}
			}
		})
	}
}
