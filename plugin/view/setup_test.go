package view

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		progCount int
	}{
		{"view example {\n expr name() == 'example.com.'\n}", false, 1},
		{"view example {\n expr incidr(client_ip(), '10.0.0.0/24')\n}", false, 1},
		{"view example {\n expr name() == 'example.com.'\n expr name() == 'example2.com.'\n}", false, 2},
		{"view", true, 0},
		{"view example {\n expr invalid expression\n}", true, 0},
	}

	for i, test := range tests {
		v, err := parse(caddy.NewTestController("dns", test.input))

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}
		if err != nil && !test.shouldErr {
			t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}
		if test.shouldErr {
			continue
		}
		if test.progCount != len(v.progs) {
			t.Errorf("Test %d: Expected prog length %d, but got %d for %s.", i, test.progCount, len(v.progs), test.input)
		}
	}
}
