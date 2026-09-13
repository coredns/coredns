package sazu

import (
	"crypto"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// SignZoneContent produces an RFC 4034 RRSIG for every RRset in rrs
// (grouped by owner name + type), signed with signer -- SAZU's central
// decision (§9.1): the one key that authenticates a push over SIG(0) is
// the same key that signs the zone content itself, so there is no
// separate DNSSEC signing step or key to manage. The DNSKEY RRset gets
// signed exactly the same way as everything else here (a self-signature,
// since it's just another RRset in rrs), which is why this package's
// single-key model doesn't need a KSK/ZSK split to produce a validly
// self-signed DNSKEY RRset.
//
// Returns rrs unchanged plus one RRSIG per distinct (name, type) group,
// in the order those groups first appeared. Pass a signer/dnskeyRR pair
// that actually match (dnskeyRR.KeyTag() must equal what signer will
// produce) -- this function does not check that itself; VerifySignedRRsets
// is what a receiver uses to confirm it.
func SignZoneContent(rrs []dns.RR, dnskeyRR *dns.DNSKEY, signer crypto.Signer, inception, expiration time.Time) ([]dns.RR, error) {
	groups := groupRRsets(rrs)
	out := make([]dns.RR, 0, len(rrs)+len(groups))
	for _, group := range groups {
		out = append(out, group...)
		sig, err := signOneRRset(group, dnskeyRR, signer, inception, expiration)
		if err != nil {
			return nil, err
		}
		out = append(out, sig)
	}
	return out, nil
}

// rrsetKey identifies one RRset: owner name (lowercased -- names are
// already stored lowercase throughout this package, but grouping is
// cheap insurance against a caller that didn't) plus type.
type rrsetKey struct {
	name  string
	rtype uint16
}

// groupRRsets partitions rrs into RRsets by (owner name, type), preserving
// the order each group first appears in -- signing (and later, serving)
// wants records processed one RRset at a time, not one record at a time.
func groupRRsets(rrs []dns.RR) [][]dns.RR {
	var order []rrsetKey
	groups := make(map[rrsetKey][]dns.RR)
	for _, rr := range rrs {
		h := rr.Header()
		k := rrsetKey{name: strings.ToLower(h.Name), rtype: h.Rrtype}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], rr)
	}
	result := make([][]dns.RR, len(order))
	for i, k := range order {
		result[i] = groups[k]
	}
	return result
}

// signOneRRset signs one RRset, filling in the RRSIG fields Sign itself
// doesn't derive from the RRset (TypeCovered, Labels, OrigTtl, and the
// owner/class/type of the RRSIG record are all set by Sign itself from
// rrset[0]'s header).
func signOneRRset(rrset []dns.RR, dnskeyRR *dns.DNSKEY, signer crypto.Signer, inception, expiration time.Time) (*dns.RRSIG, error) {
	sig := &dns.RRSIG{
		Algorithm:  dnskeyRR.Algorithm,
		KeyTag:     dnskeyRR.KeyTag(),
		SignerName: dnskeyRR.Hdr.Name,
		Inception:  uint32(inception.Unix()),
		Expiration: uint32(expiration.Unix()),
	}
	if err := sig.Sign(signer, rrset); err != nil {
		h := rrset[0].Header()
		return nil, fmt.Errorf("signing %s/%s: %w", h.Name, dns.TypeToString[h.Rrtype], err)
	}
	return sig, nil
}

// VerifySignedRRsets checks that every non-RRSIG Add-shaped RRset among
// ops has at least one covering RRSIG, also present in ops, that verifies
// against candidate and is within its validity window at now. This is
// §4's "Level 2 -- full verification": SIG(0) alone only proves who sent
// the update, not that the zone content it carries is itself validly
// DNSSEC-signed data, which is what actually determines whether the
// zone will validate for real resolvers once served.
//
// Delete-shaped ops are ignored -- there's no established convention for
// "signing" a deletion, and RFC 2136 combined with DNSSEC never asks for
// one. Ops are otherwise expected to be wire-accurate (Class/Rdlength
// reflecting what was actually unpacked), the same precondition
// EvaluatePrerequisites and ApplyUpdateOps already document.
func VerifySignedRRsets(candidate *dns.DNSKEY, ops []dns.RR, zclass uint16, now time.Time) error {
	adds := make([]dns.RR, 0, len(ops))
	for _, rr := range ops {
		h := rr.Header()
		if h.Class == zclass && h.Rrtype != dns.TypeRRSIG {
			adds = append(adds, rr)
		}
	}
	if len(adds) == 0 {
		return nil
	}

	sigs := make([]*dns.RRSIG, 0)
	for _, rr := range ops {
		if sig, ok := rr.(*dns.RRSIG); ok && rr.Header().Class == zclass {
			sigs = append(sigs, sig)
		}
	}

	for _, group := range groupRRsets(adds) {
		h := group[0].Header()
		if !anySignatureVerifies(group, h.Rrtype, sigs, candidate, now) {
			return fmt.Errorf("sazu: no valid RRSIG covers %s/%s", h.Name, dns.TypeToString[h.Rrtype])
		}
	}
	return nil
}

func anySignatureVerifies(rrset []dns.RR, covered uint16, sigs []*dns.RRSIG, candidate *dns.DNSKEY, now time.Time) bool {
	for _, sig := range sigs {
		if sig.TypeCovered != covered || !strings.EqualFold(sig.Hdr.Name, rrset[0].Header().Name) {
			continue
		}
		if sig.KeyTag != candidate.KeyTag() || sig.Algorithm != candidate.Algorithm {
			continue
		}
		if !sig.ValidityPeriod(now) {
			continue
		}
		if err := sig.Verify(candidate, rrset); err == nil {
			return true
		}
	}
	return false
}
