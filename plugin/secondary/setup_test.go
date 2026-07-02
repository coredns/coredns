package secondary

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/fall"
)

func TestSecondaryParse(t *testing.T) {
	tests := []struct {
		inputFileRules string
		shouldErr      bool
		transferFrom   string
		zones          []string
		fall           fall.F
		catalogZones   []string
	}{
		{
			`secondary {
				transfer from 127.0.0.1
			}`,
			false,
			"127.0.0.1:53",
			nil,
			fall.F{},
			nil,
		},
		{
			`secondary example.org {
				transfer from 127.0.0.1
			}`,
			false,
			"127.0.0.1:53",
			[]string{"example.org."},
			fall.F{},
			nil,
		},
		{
			`secondary catalog.example {
				transfer from 127.0.0.1
				catalog
			}`,
			false,
			"127.0.0.1:53",
			[]string{"catalog.example."},
			fall.F{},
			[]string{"catalog.example."},
		},
		{
			`secondary catalog.example {
				transfer from 127.0.0.1
				catalog extra
			}`,
			true,
			"",
			nil,
			fall.F{},
			nil,
		},
		{
			`secondary`,
			true,
			"",
			nil,
			fall.F{},
			nil,
		},
		{
			`secondary example.org {
				transferr from 127.0.0.1
			}`,
			true,
			"",
			nil,
			fall.F{},
			nil,
		},
		// fallthrough: bare (all zones)
		{
			`secondary {
				transfer from 127.0.0.1
				fallthrough
			}`,
			false,
			"127.0.0.1:53",
			nil,
			fall.Root,
			nil,
		},
		// fallthrough: specific zone
		{
			`secondary example.org {
				transfer from 127.0.0.1
				fallthrough example.org
			}`,
			false,
			"127.0.0.1:53",
			[]string{"example.org."},
			fall.F{Zones: []string{"example.org."}},
			nil,
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		s, f, catalogZones, err := secondaryParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}

		for i, name := range test.zones {
			if x := s.Names[i]; x != name {
				t.Fatalf("Test %d zone names don't match expected %q, but got %q", i, name, x)
			}
		}

		// This is only set *if* we have a zone (i.e. not in all tests above)
		for _, v := range s.Z {
			if x := v.TransferFrom[0]; x != test.transferFrom {
				t.Fatalf("Test %d transform from names don't match expected %q, but got %q", i, test.transferFrom, x)
			}
		}

		if !f.Equal(test.fall) {
			t.Fatalf("Test %d fallthrough not equal: expected %v, got %v", i, test.fall, f)
		}
		if len(catalogZones) != len(test.catalogZones) {
			t.Fatalf("Test %d catalog zone count mismatch: expected %d, got %d", i, len(test.catalogZones), len(catalogZones))
		}
		for _, name := range test.catalogZones {
			if _, ok := catalogZones[name]; !ok {
				t.Fatalf("Test %d expected catalog zone %q", i, name)
			}
		}
	}
}
