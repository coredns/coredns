package mdns

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestPProf(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		{`mdns`, false},
		{`mdns 1.2.3.4:1234`, false},
		{`mdns :1234`, false},
		{`mdns {}`, true},
		{`mdns /foo`, true},
		{`mdns {
            a b
        }`, true},
		{`mdns
          mdns`, true},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
		}
	}
}
