package view

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		ruleCount int
	}{
		{`view name == 'example.com.'`, false, 1},
		{`view incidr(client_ip, '10.0.0.0/24')`, false, 1},
		{`view name == 'example.com.'
view name == 'example2.com.'`, false, 2},
		{`view`, true, 0},
		{`view invalid expression`, true, 0},
	}

	for i, test := range tests {
		v, err := parse(caddy.NewTestController("dns", test.input))

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}
		if err != nil && !test.shouldErr{
			t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}
		if test.shouldErr {
			continue
		}
		if test.ruleCount != len(v.rules) {
			t.Errorf("Test %d: Expected rule length %d, but got %d for %s.", i, test.ruleCount, len(v.rules), test.input)
		}
	}
}

