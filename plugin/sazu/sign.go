package sazu

import (
	"crypto"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// SignZoneContent produces an RFC 4034 RRSIG for every RRset in rrs
// (grouped by owner name + type), signed with signer -- SAZU's original,
// still-default single-key model (§9.1): the one key that authenticates
// a push over SIG(0) is the same key that signs the zone content itself,
// so there is no separate DNSSEC signing step or key to manage. The
// DNSKEY RRset gets signed exactly the same way as everything else here
// (a self-signature, since it's just another RRset in rrs), which is why
// this single-key model doesn't need a KSK/ZSK split to produce a
// validly self-signed DNSKEY RRset. A zone that registers an optional
// ZSK (see keys.go) instead uses SignZoneContentSplit, below; this
// function is simply that one called with the same key on both sides,
// preserved as its own entry point so every existing single-key caller
// and test is completely unaffected by the ZSK addition.
//
// Returns rrs unchanged plus one RRSIG per distinct (name, type) group,
// in the order those groups first appeared. Pass a signer/dnskeyRR pair
// that actually match (dnskeyRR.KeyTag() must equal what signer will
// produce) -- this function does not check that itself; VerifySignedRRsets
// is what a receiver uses to confirm it.
func SignZoneContent(rrs []dns.RR, dnskeyRR *dns.DNSKEY, signer crypto.Signer, inception, expiration time.Time) ([]dns.RR, error) {
	return SignZoneContentSplit(rrs, dnskeyRR, signer, dnskeyRR, signer, inception, expiration)
}

// SignZoneContentSplit is SignZoneContent's ZSK-aware generalization: it
// signs the DNSKEY RRset specifically with ksk/kskSigner -- RFC 4034's
// own convention, that the key-signing key signs the key set -- and
// every other RRset with zsk/zskSigner, the key designated to sign
// ordinary zone content. Passing the same key/signer pair for both
// reproduces SignZoneContent's original behavior exactly (byte-for-byte:
// it's the same code path with the same key on both sides), which is
// what SignZoneContent itself now does. rrs need not actually contain a
// DNSKEY RRset -- an ordinary differential push signed entirely with the
// active ZSK, for instance, never does -- in which case kskSigner is
// simply never used.
func SignZoneContentSplit(rrs []dns.RR, ksk *dns.DNSKEY, kskSigner crypto.Signer, zsk *dns.DNSKEY, zskSigner crypto.Signer, inception, expiration time.Time) ([]dns.RR, error) {
	groups := groupRRsets(rrs)
	out := make([]dns.RR, 0, len(rrs)+len(groups))
	for _, group := range groups {
		out = append(out, group...)
		dnskeyRR, signer := zsk, zskSigner
		if group[0].Header().Rrtype == dns.TypeDNSKEY {
			dnskeyRR, signer = ksk, kskSigner
		}
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
// rrset[0]'s header) -- with one exception. Sign sets OrigTtl (the RDATA
// field carried inside the signed data itself, per RFC 4034 §3.1.5) but
// deliberately never touches Hdr.Ttl, the RRSIG record's own wire TTL:
// miekg/dns leaves that to the caller. RFC 4034 §3 requires it to match
// the covered RRset's TTL exactly ("the TTL value of an RRSIG RR MUST
// match the TTL value of the RRset it covers"); left unset, it silently
// defaults to zero. A zero-TTL record isn't just a hygiene nit here --
// found the hard way against a real validating resolver (Unbound):
// treating a just-received RRSIG as instantly-expired and dropping it
// from its cache mid-validation corrupts its own multi-step recursive
// validation state, producing an opaque, misleading SERVFAIL
// ("Cannot retrieve DS for signature") for answers that are otherwise
// completely valid.
func signOneRRset(rrset []dns.RR, dnskeyRR *dns.DNSKEY, signer crypto.Signer, inception, expiration time.Time) (*dns.RRSIG, error) {
	sig := &dns.RRSIG{
		Hdr:        dns.RR_Header{Ttl: rrset[0].Header().Ttl},
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
// ops has at least one covering RRSIG, also present in ops, that
// verifies against any key in candidates and is within its validity
// window at now. This is §4's "Level 2 -- full verification": SIG(0)
// alone only proves who sent the update, not that the zone content it
// carries is itself validly DNSSEC-signed data, which is what actually
// determines whether the zone will validate for real resolvers once
// served.
//
// candidates is every key currently trusted to sign content for this
// zone -- ordinarily ZoneKeys.ContentSigners(), the KSK plus any
// registered ZSKs, so a push signed by whichever of them the customer
// designated as their content signer still verifies here, not only one
// specific key.
//
// Delete-shaped ops are ignored -- there's no established convention for
// "signing" a deletion, and RFC 2136 combined with DNSSEC never asks for
// one. Ops are otherwise expected to be wire-accurate (Class/Rdlength
// reflecting what was actually unpacked), the same precondition
// EvaluatePrerequisites and ApplyUpdateOps already document.
//
// On failure, also returns a §12 SAZU status code when the failure is
// specific enough to warrant one: statusErrExpiredSignature if a
// covering RRSIG was found that is otherwise completely legitimate
// (right name, type, key tag, algorithm, and a cryptographically valid
// signature against at least one candidate) but simply falls outside
// its own inception/expiration window, as opposed to "" for the more
// generic "no valid RRSIG at all" case. Telling these apart matters
// operationally -- one means "re-sign and re-push," the other means
// something is actually wrong with the key or the content.
func VerifySignedRRsets(candidates []*dns.DNSKEY, ops []dns.RR, zclass uint16, now time.Time) (string, error) {
	adds := make([]dns.RR, 0, len(ops))
	for _, rr := range ops {
		h := rr.Header()
		if h.Class == zclass && h.Rrtype != dns.TypeRRSIG {
			adds = append(adds, rr)
		}
	}
	if len(adds) == 0 {
		return "", nil
	}

	sigs := make([]*dns.RRSIG, 0)
	for _, rr := range ops {
		if sig, ok := rr.(*dns.RRSIG); ok && rr.Header().Class == zclass {
			sigs = append(sigs, sig)
		}
	}

	for _, group := range groupRRsets(adds) {
		h := group[0].Header()
		ok, expired := anySignatureVerifies(group, h.Rrtype, sigs, candidates, now)
		if !ok {
			if expired {
				return statusErrExpiredSignature, fmt.Errorf(
					"sazu: RRSIG covering %s/%s is outside its validity window", h.Name, dns.TypeToString[h.Rrtype])
			}
			return "", fmt.Errorf("sazu: no valid RRSIG covers %s/%s", h.Name, dns.TypeToString[h.Rrtype])
		}
	}
	return "", nil
}

// anySignatureVerifies reports whether any sig in sigs both covers rrset
// and actually verifies against any key in candidates within its
// validity window (ok), and separately whether a cryptographically
// valid but expired (or not-yet-valid) match was seen along the way
// (expired) -- checked in that order (crypto first) specifically so a
// signature that fails crypto for its own reasons is never mistaken for
// merely expired.
func anySignatureVerifies(rrset []dns.RR, covered uint16, sigs []*dns.RRSIG, candidates []*dns.DNSKEY, now time.Time) (ok, expired bool) {
	for _, sig := range sigs {
		if sig.TypeCovered != covered || !strings.EqualFold(sig.Hdr.Name, rrset[0].Header().Name) {
			continue
		}
		for _, candidate := range candidates {
			if sig.KeyTag != candidate.KeyTag() || sig.Algorithm != candidate.Algorithm {
				continue
			}
			if err := sig.Verify(candidate, rrset); err != nil {
				continue
			}
			if !sig.ValidityPeriod(now) {
				expired = true
				continue
			}
			return true, false
		}
	}
	return false, expired
}
