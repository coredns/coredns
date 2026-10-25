package plugin

import (
	"fmt"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/cidr"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"

	"github.com/miekg/dns"
)

// See core/dnsserver/address.go - we should unify these two impls.

// Zones represents a lists of zone names.
type Zones []string

// Matches finds most specific zone that qname is subdomain of.
// Empty string signals not found condition.
func (z Zones) Matches(qname string) (zone string) {
	for _, zname := range z {
		cmp, nested, _ := Name(zname).CompareRelation(qname)
		if cmp == 0 {
			return zname // cannot be more specific than equal
		} else if cmp < 0 && nested {
			if zone == "" || Name(zname).Compare(zone) > 0 {
				zone = zname
			}
		}
	}
	return zone
}

// Contains checks if qname is subdomain of any zone in z.
func (z Zones) Contains(qname string) bool {
	return slices.ContainsFunc(z, func(zname string) bool {
		return Name(zname).Matches(qname)
	})
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
	cmp, nested, _ := n.CompareRelation(child)
	return cmp == 0 || cmp < 0 && nested
}

// Normalize lowercases and makes n fully qualified.
//
// Bytes outside a-zA-Z range with distinct upper and lower cases MUST be in \DDD form.
func (n Name) Normalize() string { return strings.ToLower(dns.Fqdn(string(n))) }

// CanonicalEscape brings escape sequences in line with presentation format as decoded from wire.
//
// Use on domain names supplied by user to properly compare and match against wire data.
//
//	Name(`café.example`).CanonicalEscape() == `caf\195\169.example`
//	Name(`ex\a\m\ple.`).CanonicalEscape() == `example.`
func (n Name) CanonicalEscape() (n1 string) {
	n1 = string(n)
	if n1 == "" {
		return n1
	}
	fqdn := dns.IsFqdn(n1)
	if !fqdn {
		n1 = n1 + "."
	}
	buf := make([]byte, len(n1)+1) // packing folds all escapes
	off, err := dns.PackDomainName(n1, buf, 0, nil, false)
	if err != nil {
		return string(n)
	}
	out, _, err := dns.UnpackDomainName(buf[:off], 0)
	if err != nil {
		return string(n)
	}
	if !fqdn {
		out = out[:len(out)-1]
	}
	return out
}

// Compare compares domain names regardless of normalization.
//
// Follows DNSSEC canonical ordering (RFC 4034, Section 6.1):
//   - `\DDD` byte is decoded before comparison
//   - Uppercase A-Z letters are treated as if they were lowercase
//   - Absence of octet sorts before zero value octet
//
// Quirks:
//   - Trailing `\` that escapes nothing is ignored
//   - Leading `\` in `\D` and `\DD` is ignored
//   - Non-FQDN names are assumed to be root-terminated
func (n Name) Compare(name string) int {
	cmp, _, _ := n.CompareRelation(name)
	return cmp
}

// CompareRelation is like [Name.Compare] but also reports how names relate:
//   - nested is whether greater name is subdomain
//   - n[ni:] is longest common label suffix of n and name
func (n Name) CompareRelation(name string) (cmp int, nested bool, ni int) {
	var (
		a, b         = string(n), name
		adot, bdot   int
		aoff, boff   int
		alast, blast = stripTrailingBackslash(a), stripTrailingBackslash(b)
		ac, bc       byte
	)

	if adot, _ = prevDot(a, alast); alast >= 0 && alast == adot {
		alast--
	}
	if bdot, _ = prevDot(b, blast); blast >= 0 && blast == bdot {
		blast--
	}
	ni = alast + 1

	//   dot       off
	//    ▼         ▼
	//  my.exampledomain.com.
	//     ▲           ▲
	//   first        last

	for alast >= 0 && blast >= 0 {
		adot, aoff = prevDot(a, alast)
		bdot, boff = prevDot(b, blast)

		for aoff <= alast && boff <= blast {
			ac, aoff = a[aoff], aoff+1
			if ac == '\\' {
				ac, aoff = nextEscapedByte(a, aoff, alast)
			}
			ac = foldCase(ac)

			bc, boff = b[boff], boff+1
			if bc == '\\' {
				bc, boff = nextEscapedByte(b, boff, blast)
			}
			bc = foldCase(bc)

			if ac != bc {
				return int(ac) - int(bc), false, ni
			}
		}

		// Shorter label means less.
		if cmp = (alast - aoff) - (blast - boff); cmp != 0 {
			return cmp, false, ni
		}

		alast = adot - 1
		blast = bdot - 1
		ni = adot + 1
	}

	// Fewer labels means less.
	return alast - blast, true, ni
}

type (
	// Host represents a host from the Corefile, may contain port.
	Host string
)

// Normalize will return the host portion of host, stripping
// of any port or transport. The host will also be fully qualified and lowercased.
// An empty string is returned on failure
// Deprecated: use OriginsFromArgsOrServerBlock or NormalizeExact
func (h Host) Normalize() string {
	var caller string
	if _, file, line, ok := runtime.Caller(1); ok {
		caller = fmt.Sprintf("(%v line %d) ", file, line)
	}
	log.Warning("An external plugin " + caller + "is using the deprecated function Normalize. " +
		"This will be removed in a future versions of CoreDNS. The plugin should be updated to use " +
		"OriginsFromArgsOrServerBlock or NormalizeExact instead.")

	s := string(h)
	_, s = parse.Transport(s)

	// The error can be ignored here, because this function is called after the corefile has already been vetted.
	hosts, _, err := SplitHostPort(s)
	if err != nil {
		return ""
	}
	return Name(hosts[0]).Normalize()
}

// MustNormalize will return the host portion of host, stripping
// of any port or transport. The host will also be fully qualified and lowercased.
// An error is returned on error
// Deprecated: use OriginsFromArgsOrServerBlock or NormalizeExact
func (h Host) MustNormalize() (string, error) {
	var caller string
	if _, file, line, ok := runtime.Caller(1); ok {
		caller = fmt.Sprintf("(%v line %d) ", file, line)
	}
	log.Warning("An external plugin " + caller + "is using the deprecated function MustNormalize. " +
		"This will be removed in a future versions of CoreDNS. The plugin should be updated to use " +
		"OriginsFromArgsOrServerBlock or NormalizeExact instead.")

	s := string(h)
	_, s = parse.Transport(s)

	// The error can be ignored here, because this function is called after the corefile has already been vetted.
	hosts, _, err := SplitHostPort(s)
	if err != nil {
		return "", err
	}
	return Name(hosts[0]).Normalize(), nil
}

// NormalizeExact will return the host portion of host, stripping
// of any port or transport. The host will also be fully qualified and lowercased.
// An empty slice is returned on failure
func (h Host) NormalizeExact() []string {
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

	if s[0] == ':' || (s[0] == '0' && strings.Contains(s, ":")) {
		return nil, "", fmt.Errorf("invalid CIDR %s", s)
	}

	// now check if multiple hosts must be returned.
	nets := cidr.Split(n)
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
			sx := Host(s[i]).NormalizeExact() // expansion of these already happened in dnsserver/register.go
			if len(sx) == 0 {
				continue
			}
			s[i] = sx[0]
		}
		return s
	}
	s := []string{}
	for i := range args {
		sx := Host(args[i]).NormalizeExact()
		if len(sx) == 0 {
			continue // silently ignores errors.
		}
		s = append(s, sx...)
	}

	return s
}

// stripTrailingBackslash removes hanging backslash that escapes nothing.
func stripTrailingBackslash(s string) (last int) {
	last = len(s) - 1
	for last >= 0 && s[last] == '\\' {
		last--
	}
	if (len(s)-last)%2 == 0 { // `...\` vs `...\\`
		return len(s) - 2
	}
	return len(s) - 1
}

// prevDot finds label-separator dot in [0, last].
func prevDot(s string, last int) (dot, first int) {
	for last >= 0 {
		if s[last] != '.' {
			last--
			continue
		}
		off1 := last - 1
		for off1 >= 0 && s[off1] == '\\' {
			off1--
		}
		if (last-off1)%2 != 0 { // `a\.example` vs `a\\.example`
			break
		}
		last = off1
	}
	return last, last + 1
}

// nextByte implements \DDD-aware and escape-aware advancement.
func nextEscapedByte(s string, off, last int) (byte, int) {
	if off+2 <= last {
		d0, d1, d2 := s[off]-'0', s[off+1]-'0', s[off+2]-'0'
		if d0 < 10 && d1 < 10 && d2 < 10 {
			return d0*100 + d1*10 + d2, off + 3
		}
	}
	return s[off], off + 1
}

func foldCase(c byte) byte {
	if c-'A' < 26 {
		c |= 0x20
	}
	return c
}
