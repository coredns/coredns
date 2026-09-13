package sazu

import (
	"fmt"
	"os"

	"github.com/miekg/dns"
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
func BuildFullZonePush(zone string, soa *dns.SOA, rrs []dns.RR, candidateKey *dns.DNSKEY, previousSOA *dns.SOA) *dns.Msg {
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
	m.Insert(adds)

	return m
}
