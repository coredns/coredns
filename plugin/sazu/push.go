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
// SAZU push (§12): the candidate DNSKEY plus every RR in the zone, guarded
// by a SOA-serial staleness prerequisite (RFC 2136 §2.4.2, "RRset exists,
// value dependent") -- the update only applies if soa is still the
// server's current SOA, so a push built from a zone snapshot the server
// has already moved past gets rejected rather than silently regressing
// it. Building this prerequisite is the client's job regardless of
// whether a given server enforces it.
//
// candidateKey is added as a DNSKEY at the zone apex -- the design's
// central decision (§9.1: the same key signs and authenticates), so it
// always travels with the push, first contact or not.
func BuildFullZonePush(zone string, soa *dns.SOA, rrs []dns.RR, candidateKey *dns.DNSKEY) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate

	m.Used([]dns.RR{soa})

	dnskeyRR := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: soa.Hdr.Ttl},
		Flags:     candidateKey.Flags,
		Protocol:  candidateKey.Protocol,
		Algorithm: candidateKey.Algorithm,
		PublicKey: candidateKey.PublicKey,
	}
	adds := make([]dns.RR, 0, len(rrs)+1)
	adds = append(adds, dnskeyRR)
	adds = append(adds, rrs...)
	m.Insert(adds)

	return m
}
