package metadata_edns0

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedLen int
	}{
		{`metadata_edns0 {
			client_id edns0 0xffed
		}`, false, 1},

		{`metadata_edns0 {
			client_id edns0
		}`, true, 1},

		{`metadata_edns0 {
			client_id edns0 0xffed
			group_id edns0 0xffee hex 16 0 16
		}`, false, 2},

		{`metadata_edns0 {
			client_id edns0 0xffed
			label edns0 0xffee
		}`, false, 2},

		{`metadata_edns0 {
			group_id edns0 
		}`, true, 1},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		actualRequest, err := parse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr && err != nil {
			continue
		}
		x := len(actualRequest.options)
		if x != test.expectedLen {
			t.Errorf("Test %v: Expected map length of %d, got: %d", i, test.expectedLen, x)
		}
	}
}
