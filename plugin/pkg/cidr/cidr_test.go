package cidr

import (
	"net"
	"testing"
)

var tests = []struct {
	in       string
	expected []string
	zones    []string
}{
	{"10.0.0.0/15", []string{"10.0.0.0/16", "10.1.0.0/16"}, []string{"0.10.in-addr.arpa.", "1.10.in-addr.arpa."}},
	{"10.0.0.0/16", []string{"10.0.0.0/16"}, []string{"0.10.in-addr.arpa."}},
	{"192.168.1.1/23", []string{"192.168.0.0/24", "192.168.1.0/24"}, []string{"0.168.192.in-addr.arpa.", "1.168.192.in-addr.arpa."}},
	{"10.129.60.0/22", []string{"10.129.60.0/24", "10.129.61.0/24", "10.129.62.0/24", "10.129.63.0/24"}, []string{"60.129.10.in-addr.arpa.", "61.129.10.in-addr.arpa.", "62.129.10.in-addr.arpa.", "63.129.10.in-addr.arpa."}},
	{"2001:db8::/31", []string{"2001:db8::/32", "2001:db9::/32"}, []string{"8.b.d.0.1.0.0.2.ip6.arpa.", "9.b.d.0.1.0.0.2.ip6.arpa."}},
}

func TestSplit(t *testing.T) {
	for i, tc := range tests {
		_, n, _ := net.ParseCIDR(tc.in)
		nets := Split(n)
		if len(nets) != len(tc.expected) {
			t.Errorf("Test %d, expected %d subnets, got %d", i, len(tc.expected), len(nets))
			continue
		}
		for j := range nets {
			if nets[j] != tc.expected[j] {
				t.Errorf("Test %d, expected %s, got %s", i, tc.expected[j], nets[j])
			}
		}
	}
}

func TestReverse(t *testing.T) {
	for i, tc := range tests {
		_, n, _ := net.ParseCIDR(tc.in)
		nets := Split(n)
		reverse := Reverse(nets)
		for j := range reverse {
			if reverse[j] != tc.zones[j] {
				t.Errorf("Test %d, expected %s, got %s", i, tc.zones[j], reverse[j])
			}
		}
	}
}
