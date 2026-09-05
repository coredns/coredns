package dynupdate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

const (
	allNames = "*"
	allTypes = "*"
)

var errMissingSOA = errors.New("zone has no SOA")

func validateRecords(records []dns.RR, origin string) error {
	origin = canonicalName(origin)
	soaCount := 0
	nameTypes := make(map[string]map[uint16]struct{})
	for _, rr := range records {
		if rr == nil {
			return errors.New("zone contains a nil record")
		}
		h := rr.Header()
		if h.Class != dns.ClassINET {
			return errors.New("zone contains a non-IN record")
		}
		if !inZone(origin, h.Name) {
			return errors.New("zone contains an out-of-zone record")
		}
		if unsupportedRRType(h.Rrtype) {
			return fmt.Errorf("zone contains unsupported RR type %s", dns.TypeToString[h.Rrtype])
		}
		name := canonicalName(h.Name)
		types := nameTypes[name]
		if types == nil {
			types = make(map[uint16]struct{})
			nameTypes[name] = types
		}
		if h.Rrtype == dns.TypeCNAME && len(types) != 0 {
			return fmt.Errorf("zone contains CNAME data conflict at %s", h.Name)
		}
		if h.Rrtype != dns.TypeCNAME {
			if _, exists := types[dns.TypeCNAME]; exists {
				return fmt.Errorf("zone contains CNAME data conflict at %s", h.Name)
			}
		}
		types[h.Rrtype] = struct{}{}
		if h.Rrtype == dns.TypeSOA {
			soa, ok := rr.(*dns.SOA)
			if !ok {
				return errors.New("zone contains an invalid SOA record")
			}
			if canonicalName(h.Name) != origin {
				return errors.New("zone contains a non-apex SOA record")
			}
			// RFC 2136 sections 4.2 and 7.11 prohibit zero for
			// interoperability with older DNS implementations.
			if soa.Serial == 0 {
				return errors.New("zone contains an SOA with serial zero")
			}
			soaCount++
		}
	}
	if soaCount == 0 {
		return errMissingSOA
	}
	if soaCount != 1 {
		return errors.New("zone contains more than one SOA record")
	}
	return nil
}

type permission struct {
	key      string
	name     string
	types    map[uint16]struct{}
	allTypes bool
}

func canonicalName(name string) string {
	return strings.ToLower(dns.CanonicalName(name))
}

func inZone(origin, name string) bool {
	name = canonicalName(name)
	return name == origin || dns.IsSubDomain(origin, name)
}

func (d *DynUpdate) configuredKey(key string) bool {
	key = canonicalName(key)
	for _, p := range d.permissions {
		if p.key == key {
			return true
		}
	}
	return false
}

func (d *DynUpdate) allows(key, name string, rrType uint16) bool {
	key = canonicalName(key)
	name = canonicalName(name)
	for _, p := range d.permissions {
		if p.key != key || (p.name != allNames && p.name != name) {
			continue
		}
		if p.allTypes {
			return true
		}
		if _, ok := p.types[rrType]; ok {
			return true
		}
	}
	return false
}

func (d *DynUpdate) nameInUse(name string) bool {
	name = canonicalName(name)
	for _, rr := range d.records {
		if canonicalName(rr.Header().Name) == name {
			return true
		}
	}
	return false
}

func (d *DynUpdate) rrset(name string, rrType uint16) []dns.RR {
	return rrsetOf(d.records, name, rrType)
}

func rrsetOf(records []dns.RR, name string, rrType uint16) []dns.RR {
	name = canonicalName(name)
	var set []dns.RR
	for _, rr := range records {
		h := rr.Header()
		if canonicalName(h.Name) == name && h.Rrtype == rrType {
			set = append(set, rr)
		}
	}
	return set
}

func (d *DynUpdate) rrsetExists(name string, rrType uint16) bool {
	return len(d.rrset(name, rrType)) != 0
}

func soaOf(records []dns.RR) *dns.SOA {
	for _, rr := range records {
		if soa, ok := rr.(*dns.SOA); ok {
			return soa
		}
	}
	return nil
}

func soaAt(records []dns.RR, name string) *dns.SOA {
	name = canonicalName(name)
	for _, rr := range records {
		soa, ok := rr.(*dns.SOA)
		if ok && canonicalName(soa.Header().Name) == name {
			return soa
		}
	}
	return nil
}

func cloneRecords(records []dns.RR) []dns.RR {
	cloned := make([]dns.RR, len(records))
	for i, rr := range records {
		cloned[i] = dns.Copy(rr)
	}
	return cloned
}

func sameRR(a, b dns.RR) bool {
	if a == nil || b == nil {
		return false
	}

	// dns.IsDuplicate implements miekg/dns's generated, type-aware RDATA
	// comparison and deliberately ignores TTL. UPDATE delete records use
	// CLASS NONE on the wire, while the corresponding zone RR has the zone
	// class, so normalize both classes before comparing.
	left, right := dns.Copy(a), dns.Copy(b)
	if left == nil || right == nil {
		return false
	}
	left.Header().Class = dns.ClassINET
	right.Header().Class = dns.ClassINET
	return dns.IsDuplicate(left, right)
}

func sameRRset(have, want []dns.RR) bool {
	// Treat each section as a set. Zone files and UPDATE messages should not
	// contain duplicate RRs, but ignoring duplicates here follows the RFC's
	// RRset semantics and avoids making comparison depend on wire ordering.
	have = uniqueRecords(have)
	want = uniqueRecords(want)
	if len(have) != len(want) {
		return false
	}
	used := make([]bool, len(have))
	for _, wanted := range want {
		found := false
		for i, actual := range have {
			if !used[i] && sameRR(actual, wanted) {
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func uniqueRecords(records []dns.RR) []dns.RR {
	unique := make([]dns.RR, 0, len(records))
	for _, rr := range records {
		duplicate := false
		for _, existing := range unique {
			if sameRR(existing, rr) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, rr)
		}
	}
	return unique
}

func findRR(records []dns.RR, want dns.RR) int {
	for i, rr := range records {
		if sameRR(rr, want) {
			return i
		}
	}
	return -1
}

func removeRecords(records []dns.RR, match func(dns.RR) bool) ([]dns.RR, bool) {
	result := make([]dns.RR, 0, len(records))
	changed := false
	for _, rr := range records {
		if match(rr) {
			changed = true
			continue
		}
		result = append(result, rr)
	}
	return result, changed
}

func countRRset(records []dns.RR, name string, rrType uint16) int {
	name = canonicalName(name)
	count := 0
	for _, rr := range records {
		if canonicalName(rr.Header().Name) == name && rr.Header().Rrtype == rrType {
			count++
		}
	}
	return count
}

func hasCNAME(records []dns.RR, name string) bool {
	return countRRset(records, name, dns.TypeCNAME) != 0
}

func hasOtherData(records []dns.RR, name string) bool {
	name = canonicalName(name)
	for _, rr := range records {
		if canonicalName(rr.Header().Name) != name {
			continue
		}
		if !cnameCompatibleType(rr.Header().Rrtype) {
			return true
		}
	}
	return false
}

func cnameCompatibleType(rrType uint16) bool {
	return rrType == dns.TypeCNAME
}

// serialGreater implements RFC 1982 serial arithmetic. The half-space value
// is deliberately not considered greater because RFC 1982 leaves it
// undefined.
func serialGreater(a, b uint32) bool {
	return a != b && a-b < 1<<31
}

func bumpSerial(records []dns.RR) {
	if soa := soaOf(records); soa != nil {
		soa.Serial++
		// RFC 2136 section 7.11 recommends that an automatically incremented
		// serial never become zero after wrapping at 2^32.
		if soa.Serial == 0 {
			soa.Serial = 1
		}
	}
}

func knownRRType(rrType uint16) bool {
	_, ok := dns.TypeToRR[rrType]
	return ok
}

func isQueryMetaType(rrType uint16) bool {
	switch rrType {
	case dns.TypeANY, dns.TypeAXFR, dns.TypeIXFR, dns.TypeMAILA, dns.TypeMAILB,
		dns.TypeOPT, dns.TypeTKEY, dns.TypeTSIG:
		return true
	default:
		return false
	}
}
