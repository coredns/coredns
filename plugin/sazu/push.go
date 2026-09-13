package sazu

import (
	"crypto"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

// Default RRSIG validity window for content this package signs.
// Independent of, and much longer than, a SIG(0) transaction signature's
// own inception/expiration (typically ~1 hour, protecting the UPDATE
// message itself against replay): this window protects the *zone
// content* — how long it stays validly signed once served, which is
// what a customer's re-signing schedule needs to stay ahead of.
const (
	DefaultSignatureInceptionSkew = 1 * time.Hour
	DefaultSignatureValidity      = 30 * 24 * time.Hour
)

// LoadZoneFile parses a BIND-format zone file and returns its SOA record
// and every other RR it contains. A full-zone SAZU push (§12) sends the
// customer's own zone content exactly as they maintain it -- not a
// synthetic subset built record by record, the way the earlier sazuctl
// push command demonstrated the protocol with a single A record.
func LoadZoneFile(path, origin string) (soa *dns.SOA, rrs []dns.RR, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	zp := dns.NewZoneParser(f, dns.Fqdn(origin), path)
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		if s, isSOA := rr.(*dns.SOA); isSOA {
			if soa != nil {
				return nil, nil, fmt.Errorf("%s: more than one SOA record", path)
			}
			soa = s
			continue
		}
		rrs = append(rrs, rr)
	}
	if err := zp.Err(); err != nil {
		return nil, nil, err
	}
	if soa == nil {
		return nil, nil, fmt.Errorf("%s: no SOA record found", path)
	}
	return soa, rrs, nil
}

// BuildFullZonePush builds an RFC 2136 UPDATE message for a full-zone
// SAZU push (§12): the candidate DNSKEY plus every record in the zone
// (soa included, so the pushed SOA actually becomes part of what the
// server serves -- it is content, not just a version marker).
//
// previousSOA, if non-nil, adds an RFC 2136 §2.4.2 "RRset exists, value
// dependent" prerequisite against it -- the SOA-serial staleness guard:
// the update only applies if previousSOA is still the server's current
// SOA, so a push built from a zone snapshot the server has already moved
// past gets rejected rather than silently regressing it. Pass nil for a
// first-contact push: there is no previously-published SOA yet to be
// stale against, and §10.2 explicitly carries no prerequisites of its
// own at first contact.
//
// candidateKey is added as a DNSKEY at the zone apex -- the design's
// central decision (§9.1: the same key signs and authenticates), so it
// always travels with the push, first contact or not.
//
// signer is that same key's private half, used to actually sign the
// pushed content (DNSKEY, SOA, every RRset in rrs, and a freshly
// computed NSEC chain covering all of it -- see BuildNSECChain) with
// real RFC 4034 RRSIGs via SignZoneContent -- this is what SAZU's whole
// premise ("split-signing DNSSEC," the hoster never touches a private
// key) actually requires: SIG(0) alone only authenticates the push
// *transaction*, not the zone *content*. Without this, a validating
// resolver would see a zone with a published DS but no RRSIGs at all —
// exactly the "bogus" state that produces SERVFAIL for real DNSSEC
// clients, regardless of whether the push mechanics themselves are sound.
//
// A full push is the only kind that ever computes or sends NSEC records
// -- it's the only one that sees the zone's entire name set at once,
// which a correct chain needs. The server invalidates any existing chain
// before applying a push that doesn't include one (see
// ZoneData.PurgeNSEC); this one always does.
func BuildFullZonePush(zone string, soa *dns.SOA, rrs []dns.RR, candidateKey *dns.DNSKEY, signer crypto.Signer, previousSOA *dns.SOA) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate

	if previousSOA != nil {
		m.Used([]dns.RR{previousSOA})
	}

	dnskeyRR := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: soa.Hdr.Ttl},
		Flags:     candidateKey.Flags,
		Protocol:  candidateKey.Protocol,
		Algorithm: candidateKey.Algorithm,
		PublicKey: candidateKey.PublicKey,
	}
	adds := make([]dns.RR, 0, len(rrs)+2)
	adds = append(adds, dnskeyRR, soa)
	adds = append(adds, rrs...)
	adds = append(adds, BuildNSECChain(soa, adds)...)

	now := time.Now()
	signed, err := SignZoneContent(adds, dnskeyRR, signer, now.Add(-DefaultSignatureInceptionSkew), now.Add(DefaultSignatureValidity))
	if err != nil {
		return nil, err
	}
	m.Insert(signed)

	return m, nil
}
