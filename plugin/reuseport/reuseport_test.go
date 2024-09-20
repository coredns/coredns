package reuseport

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestReuseport(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedRoot       string // expected root, set to the controller. Empty for negative cases.
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{`reuseport 2`, false, "", ""},
		{` reuseport 1`, false, "", ""},
		{`reuseport text`, true, "", "invalid num socks"},
		{`reuseport 0`, true, "", "num socks can not be zero or negative"},
		{`reuseport -1`, true, "", "num socks can not be zero or negative"},
		{`reuseport 2 2`, true, "", "Wrong argument count or unexpected line ending after '2'"},
		{`reuseport`, true, "", "Wrong argument count or unexpected line ending after 'reuseport'"},
		{`reuseport 2 {
			block
		}`, true, "", "Unexpected token '{', expecting argument"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)
		//cfg := dnsserver.GetConfig(c)

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
