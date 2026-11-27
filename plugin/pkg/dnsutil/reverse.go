package dnsutil

import (
	"net"
	"strings"
)

// ExtractAddressFromReverse turns a standard PTR reverse record name
// into an IP address. This works for ipv4 or ipv6.
//
// 54.119.58.176.in-addr.arpa. becomes 176.58.119.54. If the conversion
// fails the empty string is returned.
func ExtractAddressFromReverse(reverseName string) string {
	if s, ok := strings.CutSuffix(reverseName, IP4arpa); ok {
		return reverse(strings.Split(s, "."))
	}
	if s, ok := strings.CutSuffix(reverseName, IP6arpa); ok {
		return reverse6(s)
	}
	return ""
}

// IsReverse returns 0 is name is not in a reverse zone. Anything > 0 indicates
// name is in a reverse zone. The returned integer will be 1 for in-addr.arpa. (IPv4)
// and 2 for ip6.arpa. (IPv6).
func IsReverse(name string) int {
	if strings.HasSuffix(name, IP4arpa) {
		return 1
	}
	if strings.HasSuffix(name, IP6arpa) {
		return 2
	}
	return 0
}

func reverse(slice []string) string {
	for i := range len(slice) / 2 {
		j := len(slice) - i - 1
		slice[i], slice[j] = slice[j], slice[i]
	}
	ip := net.ParseIP(strings.Join(slice, ".")).To4()
	if ip == nil {
		return ""
	}
	return ip.String()
}

// reverse6 turns the labels of a reverse IPv6 name into an address, as in RFC 3596:
// b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2
// becomes 2001:db8::567:89ab.
func reverse6(s string) string {
	// 32 one-digit labels, least significant first:
	// digits at even offsets, dots at odd ones.
	if len(s) != 63 {
		return ""
	}
	for i := range 31 {
		if s[2*i+1] != '.' {
			return ""
		}
	}
	var ip [16]byte
	for i := range ip {
		lo, hi := unhex(s[4*i]), unhex(s[4*i+2])
		if lo|hi > 0xf {
			return ""
		}
		ip[15-i] = hi<<4 | lo
	}
	return net.IP(ip[:]).String()
}

// unhex returns the value of the hex digit c, or 0xff if c is not one.
func unhex(c byte) byte {
	if d := c - '0'; d < 10 {
		return d
	}
	// c|0x20 maps 'A'-'F' to 'a'-'f', and no other byte into that range.
	if d := (c | 0x20) - 'a'; d < 6 {
		return d + 10
	}
	return 0xff
}

const (
	// IP4arpa is the reverse tree suffix for v4 IP addresses.
	IP4arpa = ".in-addr.arpa."
	// IP6arpa is the reverse tree suffix for v6 IP addresses.
	IP6arpa = ".ip6.arpa."
)
