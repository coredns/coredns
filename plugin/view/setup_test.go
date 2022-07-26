package view

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		{`view name == 'example.com.'`, false},
		{`view incidr(client_ip, '10.0.0.0/24')`, false},
		{`view`, true},
		{`view invalid expression`, true},
	}

	for i, test := range tests {
		_, err := parse(caddy.NewTestController("dns", test.input))

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

