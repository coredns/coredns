package external

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/fall"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input               string
		shouldErr           bool
		expectedZone        string
		expectedApex        string
		expectedHeadless    bool
		expectedFallthrough fall.F
	}{
		{`k8s_external`, false, "", "dns", false, fall.Zero},
		{`k8s_external example.org`, false, "example.org.", "dns", false, fall.Zero},
		{`k8s_external example.org {
			apex testdns
}`, false, "example.org.", "testdns", false, fall.Zero},
		{`k8s_external example.org {
	headless
}`, false, "example.org.", "dns", true, fall.Zero},
		{`k8s_external example.org {
	fallthrough
}`, false, "example.org.", "dns", false, fall.Root},
		{`k8s_external example.org {
	fallthrough ip6.arpa inaddr.arpa foo.com
}`, false, "example.org.", "dns", false,
			fall.F{Zones: []string{"ip6.arpa.", "inaddr.arpa.", "foo.com."}}},
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

		if !test.shouldErr && test.expectedZone != "" {
			if test.expectedZone != e.Zones[0] {
				t.Errorf("Test %d, expected zone %q for input %s, got: %q", i, test.expectedZone, test.input, e.Zones[0])
			}
		}
		if !test.shouldErr {
			if test.expectedApex != e.apex {
				t.Errorf("Test %d, expected apex %q for input %s, got: %q", i, test.expectedApex, test.input, e.apex)
			}
		}
		if !test.shouldErr {
			if test.expectedHeadless != e.headless {
				t.Errorf("Test %d, expected headless %q for input %s, got: %v", i, test.expectedApex, test.input, e.headless)
			}
		}
		if !test.shouldErr {
			if !e.Fall.Equal(test.expectedFallthrough) {
				t.Errorf("Test %d, expected to be initialized with fallthrough %q for input %s, got: %v", i, test.expectedFallthrough, test.input, e.Fall)
			}
		}
	}
}
