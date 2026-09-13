package sazu

import (
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// ZoneData is one zone's live RRset store: name (lowercased FQDN) -> type
// -> RRs, plus its SOA tracked separately since queries for it are
// answered directly rather than via the generic map. Deliberately minimal
// -- SAZU needs "apply an accepted update and serve it back for testing
// the onboarding/full-push/partial-push flow end to end," not full
// authoritative fidelity (wildcards, delegation, NSEC). A production
// deployment would wire SAZU's acceptance logic into a real zone-storage
// backend instead of this self-contained one.
type ZoneData struct {
	Origin string

	mu     sync.RWMutex
	soa    *dns.SOA
	rrsets map[string]map[uint16][]dns.RR
}

// NewZoneData returns an empty zone for origin with no SOA yet -- callers
// creating a brand-new zone are expected to Insert one as part of the
// same update that creates it.
func NewZoneData(origin string) *ZoneData {
	return &ZoneData{Origin: dns.Fqdn(strings.ToLower(origin)), rrsets: make(map[string]map[uint16][]dns.RR)}
}

// SOA returns the zone's current SOA, or nil if none has been pushed yet.
func (z *ZoneData) SOA() *dns.SOA {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.soa
}

// Lookup returns a copy of the RRset for name/qtype, or nil if none.
func (z *ZoneData) Lookup(name string, qtype uint16) []dns.RR {
	z.mu.RLock()
	defer z.mu.RUnlock()
	name = strings.ToLower(name)
	if qtype == dns.TypeSOA && name == z.Origin {
		if z.soa == nil {
			return nil
		}
		return []dns.RR{dns.Copy(z.soa)}
	}
	byType, ok := z.rrsets[name]
	if !ok {
		return nil
	}
	out := make([]dns.RR, len(byType[qtype]))
	for i, rr := range byType[qtype] {
		out[i] = dns.Copy(rr)
	}
	return out
}

// LookupRRSIG returns the RRSIG(s) covering coveredType at name, if any.
// RRSIGs are stored like any other RR (under their own type, TypeRRSIG,
// in the same per-name map Lookup reads) -- including for the apex SOA,
// which is the one type Lookup itself special-cases into z.soa: a
// covering RRSIG is never diverted that way, since it isn't itself a
// *dns.SOA, so this needs no equivalent special case.
func (z *ZoneData) LookupRRSIG(name string, coveredType uint16) []dns.RR {
	z.mu.RLock()
	defer z.mu.RUnlock()
	name = strings.ToLower(name)
	byType, ok := z.rrsets[name]
	if !ok {
		return nil
	}
	var out []dns.RR
	for _, rr := range byType[dns.TypeRRSIG] {
		if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == coveredType {
			out = append(out, dns.Copy(rr))
		}
	}
	return out
}

// NameExists reports whether name has any RRset at all (including being
// the zone apex, which always "exists" once a SOA has been pushed).
func (z *ZoneData) NameExists(name string) bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	name = strings.ToLower(name)
	if name == z.Origin && z.soa != nil {
		return true
	}
	byType, ok := z.rrsets[name]
	if !ok {
		return false
	}
	for _, rrs := range byType {
		if len(rrs) > 0 {
			return true
		}
	}
	return false
}

// Insert adds rr to its RRset, per RFC 2136 §3.4.2.2 ("Add To An
// RRset"): "In case of duplicate RDATAs ... the Zone RR is replaced by
// [the] Update RR" -- an RR identical in content (ignoring TTL) to one
// already in the RRset replaces it in place rather than accumulating a
// second, redundant copy. A SOA at the zone apex replaces the tracked
// SOA outright regardless of content (RFC 1035: a zone has exactly one
// SOA) rather than appending to a list of one.
func (z *ZoneData) Insert(rr dns.RR) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.insertLocked(rr)
}

func (z *ZoneData) insertLocked(rr dns.RR) {
	if soa, ok := rr.(*dns.SOA); ok && strings.EqualFold(rr.Header().Name, z.Origin) {
		z.soa = dns.Copy(soa).(*dns.SOA)
		return
	}
	name := strings.ToLower(rr.Header().Name)
	if z.rrsets[name] == nil {
		z.rrsets[name] = make(map[uint16][]dns.RR)
	}
	existing := z.rrsets[name][rr.Header().Rrtype]

	if sig, ok := rr.(*dns.RRSIG); ok {
		z.rrsets[name][rr.Header().Rrtype] = replaceRRSIG(existing, sig)
		return
	}

	if rr.Header().Rrtype == dns.TypeNSEC {
		// Exactly one NSEC record per name, always -- unlike an ordinary
		// RRset, a differing NSEC at the same name (e.g. the zone's name
		// set changed and this name's "next" pointer needs to reflect
		// that) is a replacement, not a second value to keep alongside
		// the first.
		z.rrsets[name][rr.Header().Rrtype] = []dns.RR{dns.Copy(rr)}
		return
	}

	for i, e := range existing {
		if rrEqualContent(e, rr) {
			existing[i] = dns.Copy(rr) // replace -- refreshes TTL, per RFC 2136 §3.4.2.2
			return
		}
	}
	z.rrsets[name][rr.Header().Rrtype] = append(existing, dns.Copy(rr))
}

// replaceRRSIG adds sig to existing, first dropping any RRSIG already
// there that was produced by the same signer over the same covered type.
// RFC 2136 §3.4.2.2's "identical RDATA replaces" rule can't catch a
// re-signed RRset the way it catches an ordinary record: a fresh
// signature over unchanged content still has different RDATA (a new
// signature value, a new validity window), so by that rule alone it
// would just accumulate forever, once per re-sign, for as long as the
// zone exists -- eventually bloating every answer with expired
// signatures nothing ever removed. SAZU's single-key design (one signer
// per zone; no concurrent multi-key/algorithm rollover modeled by this
// in-memory store) means at most one active signature from a given
// signer should ever cover a given RRset at a time, so a fresh RRSIG
// from that signer replaces its own previous one instead of piling up
// beside it.
func replaceRRSIG(existing []dns.RR, sig *dns.RRSIG) []dns.RR {
	kept := existing[:0]
	for _, e := range existing {
		old, ok := e.(*dns.RRSIG)
		if ok && old.TypeCovered == sig.TypeCovered && old.Algorithm == sig.Algorithm &&
			old.KeyTag == sig.KeyTag && strings.EqualFold(old.SignerName, sig.SignerName) {
			continue
		}
		kept = append(kept, e)
	}
	return append(kept, dns.Copy(sig))
}

// DeleteRRset removes every RR of rtype at name (RFC 2136 §2.5.2).
func (z *ZoneData) DeleteRRset(name string, rtype uint16) {
	z.mu.Lock()
	defer z.mu.Unlock()
	name = strings.ToLower(name)
	if rtype == dns.TypeSOA && name == z.Origin {
		return // a zone's SOA is never removable this way, only replaced
	}
	if byType, ok := z.rrsets[name]; ok {
		delete(byType, rtype)
	}
}

// DeleteName removes every RRset at name, apex SOA excepted (RFC 2136
// §2.5.3 -- there is no protocol-level way to delete a zone this way,
// only its non-apex content).
func (z *ZoneData) DeleteName(name string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	name = strings.ToLower(name)
	if name == z.Origin {
		return
	}
	delete(z.rrsets, name)
}

// DeleteRR removes one specific RR matching rr's content -- not its TTL,
// which RFC 2136 §2.5.4 deletes ignore -- from its RRset.
func (z *ZoneData) DeleteRR(rr dns.RR) {
	z.mu.Lock()
	defer z.mu.Unlock()
	name := strings.ToLower(rr.Header().Name)
	byType, ok := z.rrsets[name]
	if !ok {
		return
	}
	rrs := byType[rr.Header().Rrtype]
	kept := rrs[:0]
	for _, existing := range rrs {
		if !rrEqualContent(existing, rr) {
			kept = append(kept, existing)
		}
	}
	byType[rr.Header().Rrtype] = kept
}

// PurgeNSEC removes every stored NSEC record (and its covering RRSIGs)
// across the whole zone. Called before applying any update (see
// handler.go's serveUpdate): SAZU's split-signing model means only a
// freshly, completely recomputed chain -- from a full push, the only
// kind that sees the zone's entire name set at once -- can be trusted as
// correct, so any existing chain is invalidated up front rather than
// risked going stale. Serving no negative-existence proof is safe;
// serving a stale one that contradicts what the zone actually contains
// now is not. A full push's own NSEC records (see BuildNSECChain)
// repopulate the chain in the same update, immediately afterward; a
// partial push that doesn't include any leaves the zone with none until
// the next full push does.
func (z *ZoneData) PurgeNSEC() {
	z.mu.Lock()
	defer z.mu.Unlock()
	for _, byType := range z.rrsets {
		delete(byType, dns.TypeNSEC)
		sigs, ok := byType[dns.TypeRRSIG]
		if !ok {
			continue
		}
		kept := sigs[:0]
		for _, rr := range sigs {
			if sig, ok := rr.(*dns.RRSIG); !ok || sig.TypeCovered != dns.TypeNSEC {
				kept = append(kept, rr)
			}
		}
		byType[dns.TypeRRSIG] = kept
	}
}

// ownerNames returns every name z holds any RRset for, including the
// apex, lowercased and deduplicated -- the node set NegativeProof's
// canonical-order search runs over.
func (z *ZoneData) ownerNames() []string {
	z.mu.RLock()
	defer z.mu.RUnlock()
	seen := map[string]bool{z.Origin: true}
	for name := range z.rrsets {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

// NegativeProof returns the NSEC record(s) (each paired with its RRSIG)
// needed to authenticate qname's negative result, per RFC 4035 §3.1.3:
//
//   - NODATA (nameExists true): just the NSEC stored at qname itself --
//     its type bitmap simply won't list the queried type, which is the
//     whole proof.
//   - NXDOMAIN (nameExists false): the NSEC covering qname itself, plus
//     the NSEC covering the wildcard slot ("*." + qname's closest
//     encloser) -- proving not only that qname doesn't exist, but that
//     no wildcard elsewhere in the zone could have matched it either.
//     SAZU never synthesizes wildcard-matched answers (see this file's
//     own top-of-file doc comment), so this is a completeness proof
//     about the zone's actual (non-wildcard) content, not a corner this
//     package cuts by ignoring wildcards it might otherwise need to
//     handle.
//
// Returns nil if the zone has no NSEC chain at all -- either nothing was
// ever pushed with one (an older push, from before this feature), or a
// partial push invalidated it (see PurgeNSEC) and no full push has
// repopulated it since. A negative response simply carries no
// authenticated denial in that case, the same as before this existed.
func (z *ZoneData) NegativeProof(qname string, nameExists bool) []dns.RR {
	qname = strings.ToLower(dns.Fqdn(qname))

	if nameExists {
		out := z.Lookup(qname, dns.TypeNSEC)
		return append(out, z.LookupRRSIG(qname, dns.TypeNSEC)...)
	}

	owners := z.ownerNames()
	if len(owners) == 0 {
		return nil
	}
	ownerSet := make(map[string]bool, len(owners))
	for _, o := range owners {
		ownerSet[o] = true
	}
	sortNamesCanonically(owners)

	var out []dns.RR
	added := make(map[string]bool, 2)
	add := func(owner string) {
		if added[owner] {
			return
		}
		added[owner] = true
		out = append(out, z.Lookup(owner, dns.TypeNSEC)...)
		out = append(out, z.LookupRRSIG(owner, dns.TypeNSEC)...)
	}

	if owner, ok := coveringOwner(qname, owners); ok {
		add(owner)
	}
	ce := closestEncloser(qname, ownerSet)
	if owner, ok := coveringOwner("*."+ce, owners); ok {
		add(owner)
	}
	return out
}

// rrEqualContent compares two RRs by name/type/rdata only, ignoring TTL
// and Class. TTL is never part of RFC 2136 delete matching. Class also
// has to be ignored here specifically because RFC 2136 §2.5.4 "delete an
// RR" (and this store's own DeleteRR caller) carries the real rdata
// alongside Class NONE as a wire-protocol marker for "this is a delete,"
// not as part of the record's identity -- the stored record being
// deleted has the zone's real class (usually IN), so comparing Class
// along with the rest would make every such delete a no-op.
func rrEqualContent(a, b dns.RR) bool {
	a2, b2 := dns.Copy(a), dns.Copy(b)
	a2.Header().Ttl, b2.Header().Ttl = 0, 0
	a2.Header().Class, b2.Header().Class = 0, 0
	return a2.String() == b2.String()
}

// Store holds every zone this plugin instance is currently serving,
// keyed by origin.
type Store struct {
	mu    sync.RWMutex
	zones map[string]*ZoneData
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{zones: make(map[string]*ZoneData)}
}

// Get returns the zone for origin, if it has been created (by a
// successful first-contact push).
func (s *Store) Get(origin string) (*ZoneData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	z, ok := s.zones[dns.Fqdn(strings.ToLower(origin))]
	return z, ok
}

// GetOrCreate returns the existing zone for origin, or creates and
// registers a new empty one.
func (s *Store) GetOrCreate(origin string) *ZoneData {
	origin = dns.Fqdn(strings.ToLower(origin))
	s.mu.Lock()
	defer s.mu.Unlock()
	if z, ok := s.zones[origin]; ok {
		return z
	}
	z := NewZoneData(origin)
	s.zones[origin] = z
	return z
}

// FindZoneForName returns the most specific onboarded zone name falls
// under, if any -- a longest-suffix match over every zone this Store
// currently holds, independent of any static configuration. This is what
// lets many customer domains be onboarded dynamically under one broad
// plugin scope (e.g. a Corefile's "sazu ."), with no per-domain Corefile
// edit needed: the set of zones this searches is whatever has actually
// been onboarded, not a fixed list. A zone with no SOA yet (shouldn't
// normally exist, given handler.go's first-contact invariant, but
// defensively excluded here too) doesn't count as found.
func (s *Store) FindZoneForName(name string) (origin string, zone *ZoneData, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best string
	var bestZone *ZoneData
	for candidate, z := range s.zones {
		if z.SOA() == nil {
			continue
		}
		if dns.IsSubDomain(candidate, name) && len(candidate) > len(best) {
			best, bestZone = candidate, z
		}
	}
	if bestZone == nil {
		return "", nil, false
	}
	return best, bestZone, true
}
