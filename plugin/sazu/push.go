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

// SynthesizeSOA builds a reasonable default SOA for a zone that doesn't
// have one yet -- for onboarding a brand-new domain without first having
// to hand-author a zone file just to get one SOA record. ns is the
// nameserver name to use as the SOA's MNAME (and, typically, the target
// of a matching NS record the caller adds separately); an empty ns
// defaults to "ns1.<zone>.". The serial is the current Unix time, which
// is simplest here since there is no previous file to read one from: a
// zone that has never been published has nothing for "the next serial"
// to be relative to.
func SynthesizeSOA(zone, ns string) *dns.SOA {
	zone = dns.Fqdn(zone)
	if ns == "" {
		ns = "ns1." + zone
	}
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      dns.Fqdn(ns),
		Mbox:    "hostmaster." + zone,
		Serial:  uint32(time.Now().Unix()),
		Refresh: 3600,
		Retry:   900,
		Expire:  604800,
		Minttl:  3600,
	}
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
// pushed content (DNSKEY, SOA, and every RRset in rrs) with real RFC 4034
// RRSIGs via SignZoneContent -- this is what SAZU's whole premise
// ("split-signing DNSSEC," the hoster never touches a private key)
// actually requires: SIG(0) alone only authenticates the push
// *transaction*, not the zone *content*. Without this, a validating
// resolver would see a zone with a published DS but no RRSIGs at all —
// exactly the "bogus" state that produces SERVFAIL for real DNSSEC
// clients, regardless of whether the push mechanics themselves are sound.
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

	now := time.Now()
	signed, err := SignZoneContent(adds, dnskeyRR, signer, now.Add(-DefaultSignatureInceptionSkew), now.Add(DefaultSignatureValidity))
	if err != nil {
		return nil, err
	}
	m.Insert(signed)

	return m, nil
}
