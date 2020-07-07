package transfer

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input     string
		zones     []string
		shouldErr bool
		exp       *Transfer
	}{
		{`transfer example.net example.org {
			to 1.2.3.4 5.6.7.8:1053 [1::2]:34
		 }
         transfer example.com example.edu {
            to * 1.2.3.4
         }`,
			nil,
			false,
			&Transfer{
				xfrs: []*xfr{{
					Zones: []string{"example.net.", "example.org."},
					to:    []string{"1.2.3.4:53", "5.6.7.8:1053", "[1::2]:34"},
				}, {
					Zones: []string{"example.com.", "example.edu."},
					to:    []string{"*", "1.2.3.4:53"},
				}},
			},
		},
		// errors
		{`transfer example.net example.org {
		 }`,
			nil,
			true,
			nil,
		},
		{`transfer example.net example.org {
           invalid option
		 }`,
			nil,
			true,
			nil,
		},
		{
			`
         transfer example.com example.edu {
            to example.com 1.2.3.4
         }`,
			nil,
			true,
			nil,
		},
		{
			`transfer {
			to 1.2.3.4 5.6.7.8:1053 [1::2]:34
		 }`,
			[]string{"."},
			false,
			&Transfer{
				xfrs: []*xfr{{
					Zones: []string{"."},
					to:    []string{"1.2.3.4:53", "5.6.7.8:1053", "[1::2]:34"},
				}},
			},
		},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		c.ServerBlockKeys = append(c.ServerBlockKeys, tc.zones...)

		transfer, err := parseTransfer(c)

		if err == nil && tc.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		}
		if err != nil && !tc.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}
		if tc.exp == nil && transfer != nil {
			t.Fatalf("Test %d expected %v xfrs, got %#v", i, tc.exp, transfer)
		}
		if tc.shouldErr {
			continue
		}

		if len(tc.exp.xfrs) != len(transfer.xfrs) {
			t.Fatalf("Test %d expected %d xfrs, got %d", i, len(tc.exp.xfrs), len(transfer.xfrs))
		}
		for j, x := range transfer.xfrs {
			// Check Zones
			if len(tc.exp.xfrs[j].Zones) != len(x.Zones) {
				t.Fatalf("Test %d expected %d zones, got %d", i, len(tc.exp.xfrs[i].Zones), len(x.Zones))
			}
			for k, zone := range x.Zones {
				if tc.exp.xfrs[j].Zones[k] != zone {
					t.Errorf("Test %d expected zone %v, got %v", i, tc.exp.xfrs[j].Zones[k], zone)

				}
			}
			// Check to
			if len(tc.exp.xfrs[j].to) != len(x.to) {
				t.Fatalf("Test %d expected %d 'to' values, got %d", i, len(tc.exp.xfrs[i].to), len(x.to))
			}
			for k, to := range x.to {
				if tc.exp.xfrs[j].to[k] != to {
					t.Errorf("Test %d expected %v in 'to', got %v", i, tc.exp.xfrs[j].to[k], to)

				}
			}
		}
	}
}

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", "transfer")
	if err := setup(c); err == nil {
		t.Fatal("Expected errors, but got nil")
	}

	c = caddy.NewTestController("dns", `transfer example.net example.org {
			to 1.2.3.4 5.6.7.8:1053 [1::2]:34
		 }
         transfer example.com example.edu {
            to * 1.2.3.4
         }`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got %v", err)
	}
}
