package autopath

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/mholt/caddy"
)

func TestSetupAutoPath(t *testing.T) {
	resolv, rm, err := test.TempFile(os.TempDir(), resolvConf)
	if err != nil {
		t.Fatalf("Could not create resolv.conf test file %s: %s", resolvConf, err)
	}
	defer rm()

	tests := []struct {
		input              string
		shouldErr          bool
		expectedZones      []string
		expectedMw         string   // expected plugin.
		expectedSearch     []string // expected search path
		expectedErrContent string   // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{`autopath @kubernetes`, false, []string{}, "kubernetes", nil, ""},
		{`autopath example.org @kubernetes`, false, []string{"example.org."}, "kubernetes", nil, ""},
		{`autopath 10.0.0.0/31 @kubernetes`, false, []string{"0.0.0.10.in-addr.arpa.", "1.0.0.10.in-addr.arpa."}, "kubernetes", nil, ""},
		{`autopath ` + resolv, false, []string{}, "", []string{"bar.com.", "baz.com.", ""}, ""},
		// negative
		{`autopath kubernetes`, true, []string{}, "", nil, "open kubernetes: no such file or directory"},
		{`autopath`, true, []string{}, "", nil, "no resolv-conf"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		ap, mw, err := autoPathParse(c)

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

		if !test.shouldErr && mw != test.expectedMw {
			t.Errorf("Test %d: Plugin not correctly set for input %s. Expected: %s, actual: %s", i, test.input, test.expectedMw, mw)
		}
		if !test.shouldErr && ap.search != nil {
			if !reflect.DeepEqual(test.expectedSearch, ap.search) {
				t.Errorf("Test %d: wrong searchpath for input %s. Expected: '%v', actual: '%v'", i, test.input, test.expectedSearch, ap.search)
			}
		}
		if !test.shouldErr {
			for j, zone := range ap.Zones {
				if test.expectedZones[j] != zone {
					t.Errorf("Test %d: expected zone %q for input %s, got: %q", i, test.expectedZones[j], test.input, zone)
				}
			}
		}
	}
}

const resolvConf = `nameserver 1.2.3.4
domain foo.com
search bar.com baz.com
options ndots:5
`
