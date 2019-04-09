package alternate

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

func TestSetupAlternate(t *testing.T) {
	testCases := []setupTestCase{
		{
			config: `alternate REFUSED . 192.168.1.1:53`,
		},
		{
			config: `alternate SERVFAIL . 192.168.1.1:53`,
		},
		{
			config: `alternate NXDOMAIN . 192.168.1.1:53`,
		},
		{
			config: `alternate original NXDOMAIN . 192.168.1.1:53`,
		},
		{
			config:        `alternate REFUSE . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config:        `alternate SRVFAIL . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config:        `alternate NODOMAIN . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config:        `alternate original NODOMAIN . 192.168.1.1:53`,
			expectedError: `is not a valid rcode`,
		},
		{
			config: `alternate REFUSED . 192.168.1.1:53 {
						max_fails 5
						force_tcp
					}`,
		},
		{
			config:        `alternate REFUSED . abc`,
			expectedError: `not an IP address or file`,
		},
		{
			config: `alternate REFUSED . 192.168.1.1:53
					 alternate REFUSED . 192.168.1.2:53`,
			expectedError: `specified more than once`,
		},
		{
			config: `alternate REFUSED . 192.168.1.1:53
					 alternate original REFUSED . 192.168.1.2:53`,
			expectedError: `specified more than once`,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s", tc.config), func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.config)
			err := setup(c)
			if err == nil {
				if tc.expectedError != "" {
					t.Errorf("Expected error '%s', but got no error", tc.expectedError)
				}
			} else {
				if tc.expectedError == "" {
					t.Errorf("Expected no error, but got '%s'", err)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error '%s', but got '%s'", tc.expectedError, err)
				}
			}
		})
	}
}
