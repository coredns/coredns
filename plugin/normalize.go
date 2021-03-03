package plugin

import (
	"fmt"
	"math"
	"math/big"
	"net"
	"strconv"
	"strings"

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
// An empty string is returned on failure
func (h Host) Normalize() string {
	// The error can be ignored here, because this function should only be called after the corefile has already been vetted.
	host, _ := h.MustNormalize()
	return host
}

// MustNormalize will return the host portion of host, stripping
// of any port or transport. The host will also be fully qualified and lowercased.
// An error is returned on error
func (h Host) MustNormalize() (string, error) {
	s := string(h)
	_, s = parse.Transport(s)

	// The error can be ignored here, because this function is called after the corefile has already been vetted.
	host, _, _, err := SplitHostPort(s)
	if err != nil {
		return "", err
	}
	return Name(host).Normalize(), nil
}

// SplitHostPort splits s up in a host and port portion, taking reverse address notation into account.
// String the string s should *not* be prefixed with any protocols, i.e. dns://. The returned ipnet is the
// *net.IPNet that is used when the zone is a reverse and a netmask is given.
func SplitHostPort(s string) (host, port string, ipnet *net.IPNet, err error) {
	// If there is: :[0-9]+ on the end we assume this is the port. This works for (ascii) domain
	// names and our reverse syntax, which always needs a /mask *before* the port.
	// So from the back, find first colon, and then check if it's a number.
	host = s

	colon := strings.LastIndex(s, ":")
	if colon == len(s)-1 {
		return "", "", nil, fmt.Errorf("expecting data after last colon: %q", s)
	}
	if colon != -1 {
		if p, err := strconv.Atoi(s[colon+1:]); err == nil {
			port = strconv.Itoa(p)
			host = s[:colon]
		}
	}

	// TODO(miek): this should take escaping into account.
	if len(host) > 255 {
		return "", "", nil, fmt.Errorf("specified zone is too long: %d > 255", len(host))
	}

	_, d := dns.IsDomainName(host)
	if !d {
		return "", "", nil, fmt.Errorf("zone is not a valid domain name: %s", host)
	}

	// Check if it parses as a reverse zone, if so we use that. Must be fully specified IP and mask.
	ip, n, err := net.ParseCIDR(host)
	ones, bits := 0, 0
	if err == nil {
		if rev, e := dns.ReverseAddr(ip.String()); e == nil {
			ones, bits = n.Mask.Size()
			// get the size, in bits, of each portion of hostname defined in the reverse address. (8 for IPv4, 4 for IPv6)
			sizeDigit := 8
			if len(n.IP) == net.IPv6len {
				sizeDigit = 4
			}
			// Get the first lower octet boundary to see what encompassing zone we should be authoritative for.
			mod := (bits - ones) % sizeDigit
			nearest := (bits - ones) + mod
			offset := 0
			var end bool
			for i := 0; i < nearest/sizeDigit; i++ {
				offset, end = dns.NextLabel(rev, offset)
				if end {
					break
				}
			}
			host = rev[offset:]
		}
	}
	return host, port, n, nil
}

// Subnets return a slice of prefixes with the desired mask subnetted from original network
func Subnets(network *net.IPNet, newPrefixLen int) ([]net.IPNet) {
	prefixLen, _ := network.Mask.Size()
	maxSubnets := int(math.Exp2(float64(newPrefixLen)) / math.Exp2(float64(prefixLen)))
	subnetsList := []net.IPNet{net.IPNet{network.IP, net.CIDRMask(newPrefixLen , 8*len(network.IP))}}
	i := 1
	for i < maxSubnets {
		temp, _ := NextSubnet(&subnetsList[len(subnetsList)-1], newPrefixLen)
		subnetsList = append(subnetsList, *temp)
		i++
	}

	return subnetsList
}
// ClassfulFromCIDR return slice of "classful" (/8, /16, /24 or /32 only) CIDR's from any CIDR
func ClassfulFromCIDR(s string) ([]string, error) {
	_, n, err := net.ParseCIDR(s)

	var networks []net.IPNet
	var cidrs []string
	if err == nil {
		ones, _ := n.Mask.Size()
		if len(n.IP) == net.IPv6len {
			// TODO Check if any ipv6 logic need to be done
			cidrs = append(cidrs, n.String())
		} else {
			// Greater equal to class A /8
			if ones <= 8 {
				networks = Subnets(n, 8)
				// Greater equal to class B /16 (the range from /9 to /16)
			} else if ones <= 16 {
				networks = Subnets(n, 16)
				// Greater equal to class C /24 (the range from /17 to /24)
			} else if ones <= 24 {
				networks = Subnets(n, 24)
				// less than class C /24 (the range from /25 to /32)
				// TODO add RFC2317 / RFC4183 support for smaller than /24 subnets? if so change the SplitHostPort to allow less than /32 subnets
			} else if ones > 24 {
				networks = Subnets(n, 32)
			}
		}
		//cast to string
		for i := 0; i < len(networks); i++ {
			cidrs = append(cidrs, networks[i].String())
		}
	}
	return cidrs, err
}


//Functions source https://github.com/apparentlymart/go-cidr
func ipToInt(ip net.IP) (*big.Int, int) {
	val := &big.Int{}
	val.SetBytes([]byte(ip))
	if len(ip) == net.IPv4len {
		return val, 32
	} else if len(ip) == net.IPv6len {
		return val, 128
	} else {
		panic(fmt.Errorf("Unsupported address length %d", len(ip)))
	}
}

func intToIP(ipInt *big.Int, bits int) net.IP {
	ipBytes := ipInt.Bytes()
	ret := make([]byte, bits/8)
	// Pack our IP bytes into the end of the return array,
	// since big.Int.Bytes() removes front zero padding.
	for i := 1; i <= len(ipBytes); i++ {
		ret[len(ret)-i] = ipBytes[len(ipBytes)-i]
	}
	return net.IP(ret)
}

// NextSubnet returns the next available subnet of the desired mask size
// starting for the maximum IP of the offset subnet
// If the IP exceeds the maxium IP then the second return value is true
func NextSubnet(network *net.IPNet, prefixLen int) (*net.IPNet, bool) {
	_, currentLast := AddressRange(network)
	mask := net.CIDRMask(prefixLen, 8*len(currentLast))
	currentSubnet := &net.IPNet{IP: currentLast.Mask(mask), Mask: mask}
	_, last := AddressRange(currentSubnet)
	last = Inc(last)
	next := &net.IPNet{IP: last.Mask(mask), Mask: mask}
	if last.Equal(net.IPv4zero) || last.Equal(net.IPv6zero) {
		return next, true
	}
	return next, false
}

//Inc increases the IP by one this returns a new []byte for the IP
func Inc(IP net.IP) net.IP {
	incIP := make([]byte, len(IP))
	copy(incIP, IP)
	for j := len(incIP) - 1; j >= 0; j-- {
		incIP[j]++
		if incIP[j] > 0 {
			break
		}
	}
	return incIP
}

// AddressRange returns the first and last addresses in the given CIDR range.
func AddressRange(network *net.IPNet) (net.IP, net.IP) {
	// the first IP is easy
	firstIP := network.IP

	// the last IP is the network address OR NOT the mask address
	prefixLen, bits := network.Mask.Size()
	if prefixLen == bits {
		// Easy!
		// But make sure that our two slices are distinct, since they
		// would be in all other cases.
		lastIP := make([]byte, len(firstIP))
		copy(lastIP, firstIP)
		return firstIP, lastIP
	}

	firstIPInt, bits := ipToInt(firstIP)
	hostLen := uint(bits) - uint(prefixLen)
	lastIPInt := big.NewInt(1)
	lastIPInt.Lsh(lastIPInt, hostLen)
	lastIPInt.Sub(lastIPInt, big.NewInt(1))
	lastIPInt.Or(lastIPInt, firstIPInt)

	return firstIP, intToIP(lastIPInt, bits)
}
