package external

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input         string
		shouldErr     bool
		expectedZones []string
		expectedApex  string
	}{
		{`k8s_external`, false, []string{}, "dns"},
		{`k8s_external example.org`, false, []string{"example.org."}, "dns"},
		{`k8s_external example.org {
			apex testdns
}`, false, []string{"example.org."}, "testdns"},
		{`k8s_external 10.0.0.0/31`, false, []string{"0.0.0.10.in-addr.arpa.", "1.0.0.10.in-addr.arpa."}, "dns"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		e, err := parse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}
		}

		if !test.shouldErr {
			if len(e.Zones) != len(test.expectedZones) {
				t.Errorf("Test %d: Expected %d zones but got %d", i, len(test.expectedZones), len(e.Zones))
			}
			for j, actual := range e.Zones {
				if actual != test.expectedZones[j] {
					t.Errorf("Test %d: expected zone %q for input %s, got: %q", i, test.expectedZones[j], test.input, actual)
				}
			}
		}
		if !test.shouldErr {
			if test.expectedApex != e.apex {
				t.Errorf("Test %d: expected apex %q for input %s, got: %q", i, test.expectedApex, test.input, e.apex)
			}
		}
	}
}
