package forward

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mholt/caddy"
)

func TestSetupForward(t *testing.T) {
	tests := []struct {
		input            string
		shouldErr        bool
		expectedFrom     string
		expectedIgnored  []string
		expectedFails    uint32
		expectedForceTCP bool
		expectedErr      string
	}{
		// positive
		{"forward . 127.0.0.1", false, ".", nil, 2, false, ""},
		{"forward . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", nil, 2, false, ""},
		{"forward . 127.0.0.1 {\nmax_fails 3\n}\n", false, ".", nil, 3, false, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\n}\n", false, ".", nil, 2, true, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && f.from != test.expectedFrom {
			t.Errorf("Test %d: expected: %s, got: %s", i, test.expectedFrom, f.from)
		}
		if !test.shouldErr && test.expectedIgnored != nil {
			if !reflect.DeepEqual(f.ignored, test.expectedIgnored) {
				t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, f.ignored)
			}
		}
		if !test.shouldErr && f.maxfails != test.expectedFails {
			t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedFails, f.maxfails)
		}
		if !test.shouldErr && f.forceTCP != test.expectedForceTCP {
			t.Errorf("Test %d: expected: %t, got: %t", i, test.expectedForceTCP, f.forceTCP)
		}
	}
}
