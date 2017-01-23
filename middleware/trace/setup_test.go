package trace

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestTraceParse(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		addr      string
	}{
		// oks
		{`trace`, false, "localhost:9153"},
		{`trace localhost:53`, false, "localhost:53"},
		// fails
		{`trace {}`, true, ""},
		{`trace /foo`, true, ""},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		m, err := traceParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}

		if test.shouldErr {
			continue
		}

		if test.addr != m.Addr {
			t.Errorf("Test %v: Expected address %s but found: %s", i, test.addr, m.Addr)
		}
	}
}
