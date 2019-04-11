package plugin

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/miekg/dns"
)

// See core/dnsserver/address.go - we should unify these two impls.

// Zones respresents a lists of zone names.
type Zones []string

// Matches checks if qname is a subdomain of any of the zones in z.  The match
// will return the most specific zones that matches. The empty string
// signals a not found condition.
func (z Zones) Matches(qname string) string {
	zone := ""
	for _, zname := range z {
		if dns.IsSubDomain(zname, qname) {
			// We want the *longest* matching zone, otherwise we may end up in a parent
			if len(zname) > len(zone) {
				zone = zname
			}
		}
	}
	return zone
}

// Normalize fully qualifies all zones in z. The zones in Z must be domain names, without
// a port or protocol prefix.
func (z Zones) Normalize() {
	for i := range z {
		z[i] = Name(z[i]).Normalize()
	}
}

// Name represents a domain name.
type Name string

// Matches checks to see if other is a subdomain (or the same domain) of n.
// This method assures that names can be easily and consistently matched.
func (n Name) Matches(child string) bool {
	if dns.Name(n) == dns.Name(child) {
		return true
	}
	return dns.IsSubDomain(string(n), child)
}

// Normalize lowercases and makes n fully qualified.
func (n Name) Normalize() string { return strings.ToLower(dns.Fqdn(string(n))) }

type (
	// Host represents a host from the Corefile, may contain port.
	Host string
)

// Normalize will return the host portion of host, stripping
// of any port or transport. The host will also be fully qualified and lowercased.
func (h Host) Normalize() []string {
	s := string(h)
	_, s = parse.Transport(s)

	// The error can be ignore here, because this function is called after the corefile has already been vetted.
	hosts, _, _, _ := SplitHostPort(s)
	var retval []string
	for _, host := range hosts {
		retval = append(retval, Name(host).Normalize())
	}
	return retval
}

func octetToStringIPv4(b *byte) string {
	return strconv.Itoa(int(*b))
}

func quadToStringIPv6(b *byte, first bool) string {
	const hexDigit = "0123456789abcdef"
	if first {
		return string(hexDigit[*b&0x0F])
	} else {
		return string(hexDigit[*b>>4])
	}
}

func octetValue(b *byte) byte {
	return *b
}

func quadValue(b *byte, first bool) byte {
	if first {
		return (*b & 0x0F)
	} else {
		return (*b >> 4)
	}
}

// SplitHostPort splits s up in a host and port portion, taking reverse address notation into account.
// String the string s should *not* be prefixed with any protocols, i.e. dns://. The returned ipnet is the
// *net.IPNet that is used when the zone is a reverse and a netmask is given.
func SplitHostPort(s string) (hosts []string, port string, ipnet *net.IPNet, err error) {
	// If there is: :[0-9]+ on the end we assume this is the port. This works for (ascii) domain
	// names and our reverse syntax, which always needs a /mask *before* the port.
	// So from the back, find first colon, and then check if its a number.
	colon := strings.LastIndex(s, ":")
	if colon == len(s)-1 {
		return []string{""}, "", nil, fmt.Errorf("expecting data after last colon: %q", s)
	}
	host := s
	hosts = []string{host}
	if colon != -1 {
		if p, err := strconv.Atoi(s[colon+1:]); err == nil {
			port = strconv.Itoa(p)
			host = s[:colon]
			hosts = []string{host}
		}
	}

	// TODO(miek): this should take escaping into account.
	if len(host) > 255 {
		return []string{""}, "", nil, fmt.Errorf("specified zone is too long: %d > 255", len(host))
	}

	_, d := dns.IsDomainName(host)
	if !d {
		return []string{""}, "", nil, fmt.Errorf("zone is not a valid domain name: %s", host)
	}

	// Check if it parses as a reverse zone, if so we use that. Must be fully specified IP and mask.
	_, n, err := net.ParseCIDR(host) //ip is not used

	if err == nil {
		ones, bits := n.Mask.Size()
		// get the size, in bits, of each portion of hostname defined in the reverse address. (8 for IPv4, 4 for IPv6)
		sizeDigit := 8
		suffix := "in-addr.arpa."
		if len(n.IP) == net.IPv6len {
			sizeDigit = 4
			suffix = "ip6.arpa."
		}

		iLabelToVariate := ones / sizeDigit
		for i := 0; i < iLabelToVariate; i++ {
			if len(n.IP) == net.IPv6len {
				suffix = quadToStringIPv6(&n.IP[i/2], (i%2) == 1) + "." + suffix
			} else {
				suffix = octetToStringIPv4(&n.IP[i]) + "." + suffix
			}
		}

		var labelToVariate byte
		if len(n.IP) == net.IPv6len {
			labelToVariate = quadValue(&n.IP[iLabelToVariate/2], (iLabelToVariate%2) == 0)
		} else {
			labelToVariate = octetValue(&n.IP[iLabelToVariate])
		}
		hosts = []string{}
		var aHost string
		var nEntries byte
		nEntries = (1 << uint((bits-ones)%sizeDigit))
		var d byte
		for d = byte(0); d < nEntries; d++ {
			var b byte
			b = byte(labelToVariate + d)

			if ones % sizeDigit == 0 {
				aHost = suffix
			} else if len(n.IP) == net.IPv6len {
				aHost = quadToStringIPv6(&b, true) + "." + suffix
			} else {
				aHost = octetToStringIPv4(&b) + "." + suffix
			}
			hosts = append(hosts, aHost)
		}
	}
	return hosts, port, n, nil
}
