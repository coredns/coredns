package dnsutil

import (
	"encoding/binary"
	"math/rand/v2"
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestExtractAddressFromReverse(t *testing.T) {
	tests := []struct {
		reverseName     string
		expectedAddress string
	}{
		{
			"54.119.58.176.in-addr.arpa.",
			"176.58.119.54",
		},
		{
			".58.176.in-addr.arpa.",
			"",
		},
		{
			"b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.in-addr.arpa.",
			"",
		},
		{
			"b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"2001:db8::567:89ab",
		},
		{
			"d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			"54.119.58.176.ip6.arpa.",
			"",
		},
		{
			// 33 nibbles: reverse6 grouped only the first 32 and answered with
			// the address of a different host instead of rejecting the name.
			"a.b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			"::.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			// 32 labels, but one is empty, so its group is 3 nibbles short.
			".a.b.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			// Uppercase nibbles are still a valid name.
			"B.A.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.B.D.0.1.0.0.2.ip6.arpa.",
			"2001:db8::567:89ab",
		},
		{
			// A v4-mapped address prints in dotted form, as net.IP does.
			"4.0.3.0.2.0.1.0.f.f.f.f.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa.",
			"1.2.3.4",
		},
		{
			"g.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			"b-a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
			"",
		},
		{
			"NONAME",
			"",
		},
		{
			"",
			"",
		},
	}
	for i, test := range tests {
		got := ExtractAddressFromReverse(test.reverseName)
		if got != test.expectedAddress {
			t.Errorf("Test %d, expected '%s', got '%s'", i, test.expectedAddress, got)
		}
	}
}

func TestExtractAddressFromReverseRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 10000 {
		var b [16]byte
		binary.BigEndian.PutUint64(b[:8], r.Uint64())
		binary.BigEndian.PutUint64(b[8:], r.Uint64())
		want := net.IP(b[:]).String()
		name, err := dns.ReverseAddr(want)
		if err != nil {
			t.Fatal(err)
		}
		if got := ExtractAddressFromReverse(name); got != want {
			t.Fatalf("ExtractAddressFromReverse(%q) = %q, want %q", name, got, want)
		}
	}
}

func BenchmarkExtractAddressFromReverse(b *testing.B) {
	for _, bc := range []struct{ name, in string }{
		{"v4", "54.119.58.176.in-addr.arpa."},
		{"v6", "b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa."},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ExtractAddressFromReverse(bc.in)
			}
		})
	}
}

func TestIsReverse(t *testing.T) {
	tests := []struct {
		name     string
		expected int
	}{
		{"b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.", 2},
		{"d.0.1.0.0.2.in-addr.arpa.", 1},
		{"example.com.", 0},
		{"", 0},
		{"in-addr.arpa.example.com.", 0},
	}
	for i, tc := range tests {
		got := IsReverse(tc.name)
		if got != tc.expected {
			t.Errorf("Test %d, got %d, expected %d for %s", i, got, tc.expected, tc.name)
		}
	}
}
