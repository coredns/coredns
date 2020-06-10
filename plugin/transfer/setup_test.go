package transfer

import (
	"net"
	"testing"

	"github.com/caddyserver/caddy"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		exp       *Transfer
	}{
		{`transfer example.net example.org {
			to 1.2.3.4
            to 5.6.7.8:1053
            to [1::2]:34
		 }
         transfer example.com example.edu {
            to *
            to 1.2.3.4
         }`,
			false,
			&Transfer{
				xfrs: []*xfr{{
					Zones: []string{"example.net.", "example.org."},
					to:    hosts{"1.2.3.4:53": nil, "5.6.7.8:1053": nil, "[1::2]:34": nil},
				}, {
					Zones: []string{"example.com.", "example.edu."},
					to:    hosts{"*": nil, "1.2.3.4:53": nil},
				}},
			},
		},
		{`transfer example.org {
			to 1.2.3.4 notify
		 }`,
			false,
			&Transfer{
				xfrs: []*xfr{{
					Zones: []string{"example.org."},
					to:    hosts{"1.2.3.4:53": &notifyOpts{}},
				}},
			},
		},
		{`transfer example.org {
			to 1.2.3.4 notify source 5.6.7.8
		 }`,
			false,
			&Transfer{
				xfrs: []*xfr{{
					Zones: []string{"example.org."},
					to:    hosts{"1.2.3.4:53": &notifyOpts{source: &net.UDPAddr{IP: net.ParseIP("5.6.7.8")}}},
				}},
			},
		},
		// errors
		{`transfer example.net example.org {
		 }`,
			true,
			nil,
		},
		{`transfer example.net example.org {
           invalid option
		 }`,
			true,
			nil,
		},
		{`transfer example.org {
           to 1.2.3.4 invalid
		 }`,
			true,
			nil,
		},
		{`transfer example.org {
           to 1.2.3.4 notify invalid
		 }`,
			true,
			nil,
		},
		{`transfer example.org {
           to 1.2.3.4 notify source invalid
		 }`,
			true,
			nil,
		},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		transfer, err := parse(c)

		if err == nil && tc.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		}
		if err != nil && !tc.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
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
			for host, opts := range x.to {
				if tc.exp.xfrs[j].to[host] == nil && opts == nil {
					continue
				}
				if opts.source == nil && tc.exp.xfrs[j].to[host].source == nil {
					continue
				}
				if opts.source == nil && tc.exp.xfrs[j].to[host].source != nil {
					t.Fatalf("Test %d expected nil for 'to notify source', got %v", i, opts.source)
				}
				if tc.exp.xfrs[j].to[host].source.String() != opts.source.String() {
					t.Errorf("Test %d expected %v for 'to notify source', got %v", i, tc.exp.xfrs[j].to[host].source, opts.source)
				}
			}
		}
	}
}
