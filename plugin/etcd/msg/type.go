package msg

import (
	"net"
	"strings"

	"github.com/miekg/dns"
)

// HostType returns the DNS type of what is encoded in the Service Host field. We're reusing
// dns.TypeXXX to not reinvent a new set of identifiers.
//
// dns.TypeA: the service's Host field contains an A record.
// dns.TypeAAAA: the service's Host field contains an AAAA record.
// dns.TypeCNAME: the service's Host field contains a name.
// dns.TypeSRV: the service's Key begins with an underscore (_)
//
// Note that a service can double/triple as a TXT record or MX record.
func (s *Service) HostType() (what uint16, normalized net.IP) {

	ip := net.ParseIP(s.Host)

	if s.Key != "" && strings.HasPrefix(Domain(s.Key), "_") {
		return dns.TypeSRV, nil
	}

	switch {
	case ip == nil:
		return dns.TypeCNAME, nil

	case ip.To4() != nil:
		return dns.TypeA, ip.To4()

	case ip.To4() == nil:
		return dns.TypeAAAA, ip.To16()
	}
	// This should never be reached.
	return dns.TypeNone, nil
}
