package timeouts

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestTimeouts(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedRoot       string // expected root, set to the controller. Empty for negative cases.
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{`timeouts {
			read 30s
		}`, false, "", ""},
		{`timeouts {
			read 1m
			write 2m
		}`, false, "", ""},
		{` timeouts {
			idle 1h
		}`, false, "", ""},
		{`timeouts {
			read 10
			write 20
			idle 60
		}`, false, "", ""},
		// negative
		{`timeouts`, true, "", "block with no timeouts specified"},
		{`timeouts {
		}`, true, "", "block with no timeouts specified"},
		{`timeouts {
			read 10s
			giraffe 30s
		}`, true, "", "unknown option"},
		{`timeouts {
			read 10s 20s
			write 30s
		}`, true, "", "Wrong argument"},
		{`timeouts {
			write snake
		}`, true, "", "failed to parse duration"},
		{`timeouts {
			idle 0s
		}`, true, "", "needs to be between"},
		{`timeouts {
			read 48h
		}`, true, "", "needs to be between"},
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
