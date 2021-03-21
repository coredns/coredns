package dnsserver

import (
	"net"
	"testing"
)

func TestClassFromCIDR(t *testing.T) {
	tests := []struct {
		in       string
		expected []string
	}{
		{"10.0.0.0/15", []string{"10.0.0.0/16", "10.1.0.0/16"}},
		{"10.0.0.0/16", []string{"10.0.0.0/16"}},
		{"192.168.1.1/23", []string{"192.168.0.0/24", "192.168.1.0/24"}},
		{"10.129.60.0/22", []string{"10.129.60.0/24", "10.129.61.0/24", "10.129.62.0/24", "10.129.63.0/24"}},
	}
	for i, tc := range tests {
		_, n, _ := net.ParseCIDR(tc.in)
		nets := classFromCIDR(n)
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
