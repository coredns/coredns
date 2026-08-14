package timeouts

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestTimeouts(t *testing.T) {
	n128, nUnlimited := 128, -1

	tests := []struct {
		input                 string
		shouldErr             bool
		expectedRoot          string // expected root, set to the controller. Empty for negative cases.
		expectedErrContent    string // substring from the expected error. Empty for positive cases.
		expectedMaxTCPQueries *int   // expected Config.MaxTCPQueries after setup. nil means left unset.
	}{
		// positive
		{`timeouts {
			read 30s
		}`, false, "", "", nil},
		{`timeouts {
			read 1m
			write 2m
		}`, false, "", "", nil},
		{` timeouts {
			idle 1h
		}`, false, "", "", nil},
		{`timeouts {
			read 10
			write 20
			idle 60
		}`, false, "", "", nil},
		{`timeouts {
			maxtcpqueries 128
		}`, false, "", "", &n128},
		{`timeouts {
			maxtcpqueries -1
		}`, false, "", "", &nUnlimited},
		{`timeouts {
			read 10s
			maxtcpqueries 128
		}`, false, "", "", &n128},
		// negative
		{`timeouts`, true, "", "block with no timeouts specified", nil},
		{`timeouts {
		}`, true, "", "block with no timeouts specified", nil},
		{`timeouts {
			read 10s
			giraffe 30s
		}`, true, "", "unknown option", nil},
		{`timeouts {
			read 10s 20s
			write 30s
		}`, true, "", "Wrong argument", nil},
		{`timeouts {
			write snake
		}`, true, "", "failed to parse duration", nil},
		{`timeouts {
			idle 0s
		}`, true, "", "needs to be between", nil},
		{`timeouts {
			read 48h
		}`, true, "", "needs to be between", nil},
		{`timeouts {
			maxtcpqueries 0
		}`, true, "", "needs to be -1", nil},
		{`timeouts {
			maxtcpqueries -2
		}`, true, "", "needs to be -1", nil},
		{`timeouts {
			maxtcpqueries snake
		}`, true, "", "invalid value for maxtcpqueries", nil},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)
		cfg := dnsserver.GetConfig(c)

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
			continue
		}

		switch {
		case test.expectedMaxTCPQueries == nil && cfg.MaxTCPQueries != nil:
			t.Errorf("Test %d: Expected Config.MaxTCPQueries to remain unset for input %s, got %d", i, test.input, *cfg.MaxTCPQueries)
		case test.expectedMaxTCPQueries != nil:
			if cfg.MaxTCPQueries == nil {
				t.Errorf("Test %d: Expected Config.MaxTCPQueries to be %d for input %s, got unset", i, *test.expectedMaxTCPQueries, test.input)
			} else if *cfg.MaxTCPQueries != *test.expectedMaxTCPQueries {
				t.Errorf("Test %d: Expected Config.MaxTCPQueries to be %d for input %s, got %d", i, *test.expectedMaxTCPQueries, test.input, *cfg.MaxTCPQueries)
			}
		}
	}
}
