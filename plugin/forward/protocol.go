package forward

// Copied from coredns/core/dnsserver/address.go

import (
	"strings"
)

// protocol returns the protocol of the string s. The second string returns s
// with the prefix chopped off.
func protocol(s string) (Protocol, string) {
	switch {
	case strings.HasPrefix(s, _tls+"://"):
		return TLS, s[len(_tls)+3:]
	case strings.HasPrefix(s, _dns+"://"):
		return DNS, s[len(_dns)+3:]
	}
	return DNS, s
}

// A Protocol used to query DNS.
type Protocol int

// Supported protocols.
const (
	Unknown Protocol = iota
	DNS
	TLS
)

const (
	_dns = "dns"
	_tls = "tls"
)
