package metrics

import (
	"testing"

	"strings"

	"github.com/mholt/caddy"
)

func TestPrometheusParse(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		addr      string
		zones     []string
	}{
		// oks
		{`prometheus`, false, "localhost:9153", []string{}},
		{`prometheus localhost:53`, false, "localhost:53", []string{}},
		// fails
		{`prometheus {}`, true, "", []string{}},
		{`prometheus /foo`, true, "", []string{}},
		{`prometheus a b c`, true, "", []string{}},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		addr, zones, err := prometheusParse(c)
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

		if test.addr != addr {
			t.Errorf("Test %v: Expected address %s but found: %s", i, test.addr, addr)
		}
		jtzones := strings.Join(test.zones, ",")
		jzones := strings.Join(zones, ",")
		if jtzones != jzones {
			t.Errorf("Test %v: Expected zones %s but found: %s", i, jtzones, zones)
		}
	}
}
