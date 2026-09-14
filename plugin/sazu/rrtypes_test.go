package sazu

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// rrTypeCase describes one "typical" resource record type this test
// pushes, signs, serves, and verifies end to end -- both at the pure
// crypto level (SignZoneContent + RRSIG.Verify) and through a real
// server round trip (BuildFullZonePush, onboarding, a DO-bit query).
// SignZoneContent/groupRRsets never special-case a record's type -- they
// group and sign purely by (owner name, type) -- so in principle "does
// DNSSEC signing work" should already generalize to any RR type for
// free. These tests exist to lock that assumption in explicitly, per
// type, rather than leave it an unverified inference from the handful of
// types (A, SOA, NS, DNSKEY) the rest of this package's tests happen to
// exercise incidentally.
type rrTypeCase struct {
	name  string
	rr    func(zone string) dns.RR // record to push, owned somewhere under zone
	qtype uint16
	qname func(zone string) string // owner name to query for it
}

func rrTypeCases(zone string) []rrTypeCase {
	return []rrTypeCase{
		{
			name:  "A",
			rr:    func(z string) dns.RR { return testA("www."+z, net.IPv4(203, 0, 113, 10)) },
			qtype: dns.TypeA,
			qname: func(z string) string { return "www." + z },
		},
		{
			name: "AAAA",
			rr: func(z string) dns.RR {
				return &dns.AAAA{Hdr: dns.RR_Header{Name: "www." + z, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300},
					AAAA: net.ParseIP("2001:db8::10")}
			},
			qtype: dns.TypeAAAA,
			qname: func(z string) string { return "www." + z },
		},
		{
			name: "CNAME",
			rr: func(z string) dns.RR {
				return &dns.CNAME{Hdr: dns.RR_Header{Name: "alias." + z, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
					Target: "www." + z}
			},
			qtype: dns.TypeCNAME,
			qname: func(z string) string { return "alias." + z },
		},
		{
			name: "MX",
			rr: func(z string) dns.RR {
				return &dns.MX{Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 300},
					Preference: 10, Mx: "mail." + z}
			},
			qtype: dns.TypeMX,
			qname: func(z string) string { return z },
		},
		{
			name: "TXT",
			rr: func(z string) dns.RR {
				return &dns.TXT{Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 300},
					Txt: []string{"v=spf1 -all"}}
			},
			qtype: dns.TypeTXT,
			qname: func(z string) string { return z },
		},
		{
			name: "NS",
			rr: func(z string) dns.RR {
				return &dns.NS{Hdr: dns.RR_Header{Name: "delegated." + z, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 300},
					Ns: "ns1.delegated." + z}
			},
			qtype: dns.TypeNS,
			qname: func(z string) string { return "delegated." + z },
		},
		{
			name: "SRV",
			rr: func(z string) dns.RR {
				return &dns.SRV{Hdr: dns.RR_Header{Name: "_sip._tcp." + z, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 300},
					Priority: 10, Weight: 20, Port: 5060, Target: "sip." + z}
			},
			qtype: dns.TypeSRV,
			qname: func(z string) string { return "_sip._tcp." + z },
		},
		{
			name: "PTR",
			rr: func(z string) dns.RR {
				return &dns.PTR{Hdr: dns.RR_Header{Name: "ptr." + z, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 300},
					Ptr: "www." + z}
			},
			qtype: dns.TypePTR,
			qname: func(z string) string { return "ptr." + z },
		},
		{
			name: "CAA",
			rr: func(z string) dns.RR {
				return &dns.CAA{Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: 300},
					Flag: 0, Tag: "issue", Value: "letsencrypt.org"}
			},
			qtype: dns.TypeCAA,
			qname: func(z string) string { return z },
		},
		{
			name: "NAPTR",
			rr: func(z string) dns.RR {
				return &dns.NAPTR{Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeNAPTR, Class: dns.ClassINET, Ttl: 300},
					Order: 100, Preference: 10, Flags: "S", Service: "SIP+D2U", Regexp: "", Replacement: "_sip._udp." + z}
			},
			qtype: dns.TypeNAPTR,
			qname: func(z string) string { return z },
		},
		{
			name: "DNAME",
			rr: func(z string) dns.RR {
				return &dns.DNAME{Hdr: dns.RR_Header{Name: "sub." + z, Rrtype: dns.TypeDNAME, Class: dns.ClassINET, Ttl: 300},
					Target: z}
			},
			qtype: dns.TypeDNAME,
			qname: func(z string) string { return "sub." + z },
		},
	}
}

// TestSignZoneContentAndVerifyAcrossTypicalRRTypes proves the pure
// DNSSEC crypto path (SignZoneContent producing an RRSIG, RRSIG.Verify
// checking it against the served RRset) is correct for every type in
// rrTypeCases -- no network, no server, just the signing/verification
// primitives every other path in this package builds on.
func TestSignZoneContentAndVerifyAcrossTypicalRRTypes(t *testing.T) {
	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	dnskeyRR := dnskeyRRFor(key)

	for _, tc := range rrTypeCases("example.org.") {
		t.Run(tc.name, func(t *testing.T) {
			rr := tc.rr("example.org.")
			now := time.Now()
			signed, err := SignZoneContent([]dns.RR{rr}, dnskeyRR, priv, now.Add(-time.Hour), now.Add(time.Hour))
			if err != nil {
				t.Fatalf("SignZoneContent: %v", err)
			}

			var sig *dns.RRSIG
			for _, s := range signed {
				if candidate, ok := s.(*dns.RRSIG); ok {
					sig = candidate
				}
			}
			if sig == nil {
				t.Fatalf("expected an RRSIG among %+v", signed)
			}
			if sig.TypeCovered != tc.qtype {
				t.Fatalf("RRSIG.TypeCovered = %d, want %d", sig.TypeCovered, tc.qtype)
			}
			if sig.Hdr.Ttl != rr.Header().Ttl {
				t.Fatalf("RRSIG.Hdr.Ttl = %d, want %d (RFC 4034 §3: must match the covered RRset)", sig.Hdr.Ttl, rr.Header().Ttl)
			}
			if err := sig.Verify(dnskeyRR, []dns.RR{rr}); err != nil {
				t.Fatalf("RRSIG does not verify against its own covered record: %v", err)
			}

			// A tampered copy of the same record must fail verification --
			// proves the signature is actually over this record's content,
			// not just structurally present.
			tampered := dns.Copy(rr)
			switch v := tampered.(type) {
			case *dns.A:
				v.A = net.IPv4(198, 51, 100, 1)
			case *dns.AAAA:
				v.AAAA = net.ParseIP("2001:db8::99")
			case *dns.CNAME:
				v.Target = "tampered." + v.Target
			case *dns.MX:
				v.Preference = 999
			case *dns.TXT:
				v.Txt = []string{"tampered"}
			case *dns.NS:
				v.Ns = "tampered." + v.Ns
			case *dns.SRV:
				v.Port = 9999
			case *dns.PTR:
				v.Ptr = "tampered." + v.Ptr
			case *dns.CAA:
				v.Value = "tampered.example."
			case *dns.NAPTR:
				v.Order = 999
			case *dns.DNAME:
				v.Target = "tampered." + v.Target
			default:
				t.Fatalf("unhandled type in tamper switch: %T", v)
			}
			if err := sig.Verify(dnskeyRR, []dns.RR{tampered}); err == nil {
				t.Fatalf("expected a tampered %s record to fail RRSIG verification", tc.name)
			}
		})
	}
}

// TestServeAndVerifyAcrossTypicalRRTypes proves the full, real path: a
// zone carrying one record of each typical type, pushed and onboarded
// through a real dnsserver.Server exactly as a customer's own push
// would arrive, is servable with the DO bit (record plus its covering
// RRSIG, the RRSIG verifying against the zone's own published DNSKEY),
// and carries no RRSIG at all without DO -- the same "Secure" vs
// "Insecure" answer shape TestOnboardedZoneServesRRSIGsWithDOBit already
// proves for a single A record, generalized across every type in
// rrTypeCases at once via one shared zone push.
func TestServeAndVerifyAcrossTypicalRRTypes(t *testing.T) {
	s := newTestSazu("example.org.")
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	cases := rrTypeCases("example.org.")
	rrs := make([]dns.RR, 0, len(cases))
	for _, tc := range cases {
		rrs = append(rrs, tc.rr("example.org."))
	}

	soa := testSOA(1)
	push, err := BuildFullZonePush("example.org.", soa, rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if resp := sendRaw(t, addr, wire); resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("onboarding push rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qname := tc.qname("example.org.")

			withDO := queryDO(t, addr, qname, tc.qtype)
			if withDO.Rcode != dns.RcodeSuccess {
				t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[withDO.Rcode])
			}
			var covered []dns.RR
			var sig *dns.RRSIG
			for _, rr := range withDO.Answer {
				if rr.Header().Rrtype == tc.qtype {
					covered = append(covered, rr)
					continue
				}
				if s, ok := rr.(*dns.RRSIG); ok && s.TypeCovered == tc.qtype {
					sig = s
				}
			}
			if len(covered) == 0 {
				t.Fatalf("expected a %s record in the answer, got %+v", tc.name, withDO.Answer)
			}
			if sig == nil {
				t.Fatalf("expected a covering RRSIG with DO set, got %+v", withDO.Answer)
			}
			if err := sig.Verify(key, covered); err != nil {
				t.Fatalf("the served RRSIG does not verify against the served %s record: %v", tc.name, err)
			}

			withoutDO := query(t, addr, qname, tc.qtype)
			if withoutDO.Rcode != dns.RcodeSuccess {
				t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[withoutDO.Rcode])
			}
			foundRecord := false
			for _, rr := range withoutDO.Answer {
				if _, ok := rr.(*dns.RRSIG); ok {
					t.Fatalf("expected no RRSIG without the DO bit, got %+v", withoutDO.Answer)
				}
				if rr.Header().Rrtype == tc.qtype {
					foundRecord = true
				}
			}
			if !foundRecord {
				t.Fatalf("expected the %s record even without DO, got %+v", tc.name, withoutDO.Answer)
			}
		})
	}
}

// TestRequireValidRRSIGsAcceptsEveryTypicalRRType proves §4's "Level 2"
// content-signature verification (RequireValidRRSIGs) doesn't silently
// special-case which types it can check -- a push carrying every type in
// rrTypeCases, each with its own genuine RRSIG, is accepted in full.
func TestRequireValidRRSIGsAcceptsEveryTypicalRRType(t *testing.T) {
	s := newTestSazu("example.org.")
	s.RequireValidRRSIGs = true
	addr := serveThroughRealServer(t, s)

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	cases := rrTypeCases("example.org.")
	rrs := make([]dns.RR, 0, len(cases))
	for _, tc := range cases {
		rrs = append(rrs, tc.rr("example.org."))
	}
	push, err := BuildFullZonePush("example.org.", testSOA(1), rrs, key, priv, nil)
	if err != nil {
		t.Fatalf("building push: %v", err)
	}
	now := time.Now()
	wire, err := SignUpdate(push, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	resp := sendRaw(t, addr, wire)
	if resp.Rcode != dns.RcodeSuccess {
		status, _ := diagnosticStatus(resp)
		t.Fatalf("rcode = %s (status=%q), want NOERROR", dns.RcodeToString[resp.Rcode], status)
	}
}
