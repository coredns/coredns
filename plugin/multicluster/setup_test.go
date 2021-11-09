package multicluster

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"strings"
	"testing"
)

func TestParseStanza(t *testing.T) {
	tests := []struct {
		input               string // Corefile data as string
		shouldErr           bool   // true if test case is expected to produce an error.
		expectedErrContent  string // substring from the expected error. Empty for positive cases.
		expectedZoneCount   int    // expected count of defined zones.
		expectedFallthrough fall.F
	}{
		// positive
		{
			`multicluster clusterset.local`,
			false,
			"",
			1,
			fall.Zero,
		},
		{
			`multicluster coredns.local clusterset.local`,
			false,
			"",
			2,
			fall.Zero,
		},
		{
			`kubernetes coredns.local clusterset.local {
    fallthrough
}`,
			false,
			"",
			2,
			fall.Root,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		k8sController, err := ParseStanza(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error, but did not find error for input '%s'. Error was: '%v'", i, test.input, err)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
				continue
			}

			if test.shouldErr && (len(test.expectedErrContent) < 1) {
				t.Fatalf("Test %d: Test marked as expecting an error, but no expectedErrContent provided for input '%s'. Error was: '%v'", i, test.input, err)
			}

			if test.shouldErr && (test.expectedZoneCount >= 0) {
				t.Errorf("Test %d: Test marked as expecting an error, but provides value for expectedZoneCount!=-1 for input '%s'. Error was: '%v'", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err, test.input)
			}
			continue
		}

		// No error was raised, so validate initialization of k8sController
		//     Zones
		foundZoneCount := len(k8sController.Zones)
		if foundZoneCount != test.expectedZoneCount {
			t.Errorf("Test %d: Expected kubernetes controller to be initialized with %d zones, instead found %d zones: '%v' for input '%s'", i, test.expectedZoneCount, foundZoneCount, k8sController.Zones, test.input)
		}

		// fallthrough
		if !k8sController.Fall.Equal(test.expectedFallthrough) {
			t.Errorf("Test %d: Expected kubernetes controller to be initialized with fallthrough '%v'. Instead found fallthrough '%v' for input '%s'", i, test.expectedFallthrough, k8sController.Fall, test.input)
		}
	}
}
