package dynapi

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/mholt/caddy"
)

func TestSetupApi(t *testing.T) {
	zoneFileName1, rm, err := test.TempFile(".", fileMasterZone)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()
	tests := []struct {
		input     string
		shouldErr bool
	}{
		{`dynapi`, false},
		{`dynapi localhost:1234`, false},
		{`dynapi bla:a`, false},
		{`dynapi bla`, true},
		{`dynapi bla bla`, true},
		{
			`file ` + zoneFileName1 + ` example.net. {
				no_rebloat
			}`,
			true,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		_, _, err := apiParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}
		}
	}
}
