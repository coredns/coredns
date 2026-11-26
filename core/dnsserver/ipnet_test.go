package dnsserver

import "testing"

func TestServerNet(t *testing.T) {
	tests := []struct {
		netProto string
		proto    string
		expected string
	}{
		// Dual-stack (default)
		{"", "tcp", "tcp"},
		{"", "udp", "udp"},

		// IPv4-only
		{"4", "tcp", "tcp4"},
		{"4", "udp", "udp4"},

		// IPv6-only
		{"6", "tcp", "tcp6"},
		{"6", "udp", "udp6"},
	}

	for _, tc := range tests {
		t.Run(tc.proto+"_proto"+tc.netProto, func(t *testing.T) {
			s := &Server{netProto: tc.netProto}
			got := s.net(tc.proto)
			if got != tc.expected {
				t.Errorf("Server{netProto: %q}.net(%q) = %q, want %q", tc.netProto, tc.proto, got, tc.expected)
			}
		})
	}
}
