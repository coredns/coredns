package parse

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestTransferIn(t *testing.T) {
	tests := []struct {
		inputFileRules string
		shouldErr      bool
		expectedFrom   []string
	}{
		{
			`from 127.0.0.1`,
			false, []string{"127.0.0.1:53"},
		},
		// OK transfer froms
		{
			`from 127.0.0.1 127.0.0.2`,
			false, []string{"127.0.0.1:53", "127.0.0.2:53"},
		},
		// Bad transfer from garbage
		{
			`from !@#$%^&*()`,
			true, []string{},
		},
		// Bad transfer from no args
		{
			`from`,
			true, []string{},
		},
		// Bad transfer from *
		{
			`from *`,
			true, []string{},
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		froms, err := TransferIn(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error %+v %+v", i, err, test)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}

		if test.expectedFrom != nil {
			for j, got := range froms {
				if got != test.expectedFrom[j] {
					t.Fatalf("Test %d expected %v, got %v", i, test.expectedFrom[j], got)
				}
			}
		}
	}
}
