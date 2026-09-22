package dnsserver

import (
	"testing"
)

func TestRegex1035PrefSyntax(t *testing.T) {
	testCases := []struct {
		zone     string
		expected bool
	}{
		{zone: ".", expected: true},
		{zone: "example.com.", expected: true},
		{zone: "example.", expected: true},
		{zone: "example123.", expected: true},
		{zone: "example123.com.", expected: true},
		{zone: "abc-123.com.", expected: true},
		{zone: "an-example.com.", expected: true},
		{zone: "a.example.com.", expected: true},
		{zone: "1.0.0.2.ip6.arpa.", expected: true},
		{zone: "0.10.in-addr.arpa.", expected: true},
		{zone: "example", expected: false},
		{zone: "example:.", expected: false},
		{zone: "-example.com.", expected: false},
		{zone: ".example.com.", expected: false},
		{zone: "1.example.com", expected: false},
		{zone: "abc.123-xyz.", expected: false},
		{zone: "example-?&^%$.com.", expected: false},
		{zone: "abc-.example.com.", expected: false},
		{zone: "abc-%$.example.com.", expected: false},
		{zone: "123-abc.example.com.", expected: false},
	}

	for _, testCase := range testCases {
		if checkZoneSyntax(testCase.zone) != testCase.expected {
			t.Errorf("Expected %v for %q", testCase.expected, testCase.zone)
		}
	}
}

func TestStartUpZones(t *testing.T) {
	tests := []struct {
		name           string
		protocol       string
		addr           string
		zones          map[string][]*Config
		expectedOutput string
	}{
		{
			name:           "no zones",
			protocol:       "dns://",
			addr:           "127.0.0.1:53",
			zones:          map[string][]*Config{},
			expectedOutput: "",
		},
		{
			name:           "single zone valid syntax ip and port",
			protocol:       "dns://",
			addr:           "127.0.0.1:53",
			zones:          map[string][]*Config{"example.com.": nil},
			expectedOutput: "dns://example.com.:53 on 127.0.0.1\n",
		},
		{
			name:           "single zone valid syntax port only",
			protocol:       "http://",
			addr:           ":8080",
			zones:          map[string][]*Config{"example.org.": nil},
			expectedOutput: "http://example.org.:8080\n",
		},
		{
			name:     "single zone invalid syntax",
			protocol: "tls://",
			addr:     "10.0.0.1:853",
			zones:    map[string][]*Config{"invalid-zone": nil},
			expectedOutput: "Warning: Domain \"invalid-zone\" does not follow RFC1035 preferred syntax\n" +
				"tls://invalid-zone:853 on 10.0.0.1\n",
		},
		{
			name:     "multiple zones sorted order",
			protocol: "dns://",
			addr:     "localhost:5353",
			zones: map[string][]*Config{
				"c-zone.com.": nil,
				"a-zone.org.": nil,
				"b-zone.net.": nil,
			},
			expectedOutput: "dns://a-zone.org.:5353 on localhost\n" +
				"dns://b-zone.net.:5353 on localhost\n" +
				"dns://c-zone.com.:5353 on localhost\n",
		},
		{
			name:           "addr parse error",
			protocol:       "grpc://",
			addr:           "[::1]:8080:extra", // Malformed, should cause SplitProtocolHostPort to error
			zones:          map[string][]*Config{"error.example.": nil},
			expectedOutput: "grpc://error.example.:[::1]:8080:extra\n",
		},
		{
			name:           "root zone",
			protocol:       "dns://",
			addr:           "192.168.1.1:53",
			zones:          map[string][]*Config{".": nil},
			expectedOutput: "dns://.:53 on 192.168.1.1\n",
		},
		{
			name:           "reverse zone",
			protocol:       "dns://",
			addr:           ":53",
			zones:          map[string][]*Config{"1.0.168.192.in-addr.arpa.": nil},
			expectedOutput: "dns://1.0.168.192.in-addr.arpa.:53\n",
		},
		{
			name:     "multiple zones mixed syntax and addr handling",
			protocol: "quic://",
			addr:     "coolserver.local:784",
			zones: map[string][]*Config{
				"valid.net.":         nil,
				"_tcp.service.":      nil, // Invalid syntax
				"another.valid.com.": nil,
			},
			expectedOutput: "Warning: Domain \"_tcp.service.\" does not follow RFC1035 preferred syntax\n" +
				"quic://_tcp.service.:784 on coolserver.local\n" +
				"quic://another.valid.com.:784 on coolserver.local\n" +
				"quic://valid.net.:784 on coolserver.local\n",
		},
		{
			name:     "zone with leading dash invalid",
			protocol: "dns://",
			addr:     "127.0.0.1:53",
			zones:    map[string][]*Config{"-leadingdash.com.": nil},
			expectedOutput: "Warning: Domain \"-leadingdash.com.\" does not follow RFC1035 preferred syntax\n" +
				"dns://-leadingdash.com.:53 on 127.0.0.1\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := startUpZones(tc.protocol, tc.addr, tc.zones)
			if got != tc.expectedOutput {
				// Use %q for expected and got to make differences in whitespace/newlines visible.
				t.Errorf("startUpZones(%q, %q, ...) mismatch for test '%s':\nGot:\n%q\nExpected:\n%q",
					tc.protocol, tc.addr, tc.name, got, tc.expectedOutput)
			}
		})
	}
}
