package dns64

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetupDns64(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		{
			`chaos`, false,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := dns64Parse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}
		}
	}
}
