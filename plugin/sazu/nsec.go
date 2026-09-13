package sazu

import (
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// This file implements RFC 4034 authenticated denial of existence via
// NSEC -- not NSEC3. NSEC3 (RFC 5155) exists to additionally hide a
// zone's actual name set from enumeration ("zone walking"); it's a real,
// separate, opt-in enhancement, not something a correct NXDOMAIN/NODATA
// proof requires. Plain NSEC is what's needed to resolve validating
// resolvers no longer treating this server's negative answers as Bogus,
// which is the actual problem this closes.
//
// SAZU's split-signing model changes where an NSEC chain has to come
// from. A hosting provider that holds the zone's private key (e.g. AWS
// Route 53) can synthesize a covering NSEC on the fly, at answer time,
// for literally any query -- SAZU's server never holds one, so that's
// not an option here. Instead, the customer's own signer computes and
// signs a complete NSEC chain across the *whole* zone up front, as part
// of a full-zone push (see BuildNSECChain, called from
// BuildFullZonePush) -- the same way traditional offline zone-signing
// tools (e.g. dnssec-signzone) work. The server's job is limited to
// storing whatever chain it was given and, at query time, picking the
// right already-signed record out of it (see ZoneData.NegativeProof in
// store.go).
//
// A consequence of that split: only a full push can be trusted to
// produce a *complete* chain, since only it sees the zone's entire name
// set at once. A partial push (sazuctl push-update) never computes or
// includes NSEC records, so ZoneData.PurgeNSEC invalidates any existing
// chain before applying one -- serving no negative-existence proof at
// all until the next full push is a safe degradation; serving a stale
// one that contradicts what the zone actually now contains is not.

// canonicalCompare orders a and b per RFC 4034 §6.1 ("Canonical DNS Name
// Order"): labels compare from most-significant (rightmost) to least,
// case-insensitively (US-ASCII only), with a name that is a proper
// right-hand suffix of another sorting first. Returns a value whose sign
// indicates order, the way strings.Compare's does (not necessarily
// exactly -1/0/1).
//
// Implemented over dns.SplitDomainName's label strings rather than
// decoded wire bytes -- correct for the plain ASCII hostnames SAZU zones
// actually contain, but not a general-purpose implementation: a label
// using a \DDD numeric escape (RFC 1035 §5.1) for a byte outside normal
// printable ASCII compares by its literal escaped text, not the byte it
// represents, which no real SAZU deployment will ever need to push.
func canonicalCompare(a, b string) int {
	la, lb := reversedLabels(a), reversedLabels(b)
	for i := 0; i < len(la) && i < len(lb); i++ {
		if c := strings.Compare(la[i], lb[i]); c != 0 {
			return c
		}
	}
	return len(la) - len(lb)
}

func reversedLabels(name string) []string {
	labels := dns.SplitDomainName(dns.Fqdn(name))
	out := make([]string, len(labels))
	for i, l := range labels {
		out[len(labels)-1-i] = strings.ToLower(l)
	}
	return out
}

// sortNamesCanonically sorts names in place in RFC 4034 §6.1 order.
func sortNamesCanonically(names []string) {
	sort.Slice(names, func(i, j int) bool { return canonicalCompare(names[i], names[j]) < 0 })
}

// closestEncloser returns the longest suffix of qname present in owners
// (a set of lowercased FQDNs) -- RFC 4035 §3.1.3's "closest encloser,"
// used to locate the wildcard slot ("*."+closest encloser) that also has
// to be proven nonexistent for a complete NXDOMAIN answer. Terminates
// unconditionally as long as some suffix of qname (at minimum, the
// zone's own apex) is in owners, which NegativeProof's caller guarantees.
func closestEncloser(qname string, owners map[string]bool) string {
	labels := dns.SplitDomainName(dns.Fqdn(qname))
	for i := 0; i <= len(labels); i++ {
		candidate := dns.Fqdn(strings.ToLower(strings.Join(labels[i:], ".")))
		if owners[candidate] {
			return candidate
		}
	}
	return "" // unreachable given the guarantee above
}

// coveringOwner returns which member of sortedOwners (already in
// RFC 4034 canonical order, deduplicated) holds the NSEC record covering
// name: the largest owner canonically less than name, or -- since the
// NSEC chain is circular -- the last owner in the chain if name
// canonically precedes every one of them. ok is false only when
// sortedOwners is empty (no NSEC chain exists for this zone at all).
func coveringOwner(name string, sortedOwners []string) (owner string, ok bool) {
	if len(sortedOwners) == 0 {
		return "", false
	}
	name = strings.ToLower(dns.Fqdn(name))
	best := -1
	for i, o := range sortedOwners {
		if canonicalCompare(o, name) < 0 {
			best = i
			continue
		}
		break // sortedOwners is ascending -- nothing further can improve "best"
	}
	if best == -1 {
		return sortedOwners[len(sortedOwners)-1], true // wraps around the end of the chain
	}
	return sortedOwners[best], true
}

// BuildNSECChain synthesizes one RFC 4034 §4 NSEC record per distinct
// owner name in adds (a full push's complete content -- DNSKEY, SOA, and
// every zone record, exactly what BuildFullZonePush is about to sign and
// send), covering the whole zone. adds is read-only; the returned
// records still need signing like everything else, which
// BuildFullZonePush does by simply including them in the same
// SignZoneContent call as the rest.
//
// Each record's Next Domain Name is the next owner in RFC 4034 §6.1
// canonical order, wrapping back to the first owner after the last --
// the chain is circular, per RFC 4034 §4's definition. Its type bitmap
// lists every RRset type actually present at that name, plus NSEC and
// RRSIG themselves (both of which will exist there once this is signed
// and inserted). Its TTL is the zone's SOA minimum, per RFC 4034 §4's
// explicit requirement -- not each name's own record TTL, the way every
// other RRset in this package uses.
func BuildNSECChain(soa *dns.SOA, adds []dns.RR) []dns.RR {
	apex := dns.Fqdn(soa.Hdr.Name)
	typesByName := map[string]map[uint16]bool{strings.ToLower(apex): {}}
	for _, rr := range adds {
		name := strings.ToLower(dns.Fqdn(rr.Header().Name))
		if typesByName[name] == nil {
			typesByName[name] = make(map[uint16]bool)
		}
		typesByName[name][rr.Header().Rrtype] = true
	}

	names := make([]string, 0, len(typesByName))
	for name := range typesByName {
		names = append(names, name)
	}
	sortNamesCanonically(names)

	out := make([]dns.RR, 0, len(names))
	for i, name := range names {
		types := typesByName[name]
		types[dns.TypeNSEC] = true
		types[dns.TypeRRSIG] = true
		bitmap := make([]uint16, 0, len(types))
		for t := range types {
			bitmap = append(bitmap, t)
		}
		sort.Slice(bitmap, func(a, b int) bool { return bitmap[a] < bitmap[b] }) // packDataNsec requires ascending order

		out = append(out, &dns.NSEC{
			Hdr:        dns.RR_Header{Name: name, Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: soa.Minttl},
			NextDomain: names[(i+1)%len(names)],
			TypeBitMap: bitmap,
		})
	}
	return out
}
