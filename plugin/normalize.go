package plugin

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/cidr"
	"github.com/coredns/coredns/plugin/pkg/parse"

	"github.com/miekg/dns"
)

// See core/dnsserver/address.go - we should unify these two impls.

// Zones represents a lists of zone names.
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
// An empty slice is returned on failure
func (h Host) Normalize() []string {
	// The error can be ignored here, because this function should only be called after the corefile has already been vetted.
	s := string(h)
	_, s = parse.Transport(s)

	hosts, _, err := SplitHostPort(s)
	if err != nil {
		return nil
	}
	for i := range hosts {
		hosts[i] = Name(hosts[i]).Normalize()

	}
	return hosts
}

// SplitHostPort splits s up in a host(s) and port portion, taking reverse address notation into account.
// String the string s should *not* be prefixed with any protocols, i.e. dns://. SplitHostPort can return
// multiple hosts when a reverse notation on a non-octet boundary is given.
func SplitHostPort(s string) (hosts []string, port string, err error) {
	// If there is: :[0-9]+ on the end we assume this is the port. This works for (ascii) domain
	// names and our reverse syntax, which always needs a /mask *before* the port.
	// So from the back, find first colon, and then check if it's a number.
	colon := strings.LastIndex(s, ":")
	if colon == len(s)-1 {
		return nil, "", fmt.Errorf("expecting data after last colon: %q", s)
	}
	if colon != -1 {
		if p, err := strconv.Atoi(s[colon+1:]); err == nil {
			port = strconv.Itoa(p)
			s = s[:colon]
		}
	}

	// TODO(miek): this should take escaping into account.
	if len(s) > 255 {
		return nil, "", fmt.Errorf("specified zone is too long: %d > 255", len(s))
	}

	if _, ok := dns.IsDomainName(s); !ok {
		return nil, "", fmt.Errorf("zone is not a valid domain name: %s", s)
	}

	// Check if it parses as a reverse zone, if so we use that. Must be fully specified IP and mask.
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return []string{s}, port, nil
	}

	// now check if multiple hosts must be returned.
	nets := cidr.Class(n)
	hosts = cidr.Reverse(nets)
	return hosts, port, nil
}

// OriginsFromArgsOrServerBlock returns the normalized args if that slice
// is not empty, otherwise the serverblock slice is returned (in a newly copied slice).
func OriginsFromArgsOrServerBlock(args, serverblock []string) []string {
	if len(args) == 0 {
		s := make([]string, len(serverblock))
		copy(s, serverblock)
		for i := range s {
			s[i] = Host(s[i]).Normalize()[0] // expansion of these already happened in dnsserver/registrer.go
		}
		return s
	}
	s := []string{}
	for i := range args {
		s = append(s, Host(args[i]).Normalize()...)
	}

	return s
}
