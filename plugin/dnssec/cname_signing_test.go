package dnssec

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// numSigs counts the RRSIG records in a message section.
func numSigs(rrs []dns.RR) int {
	n := 0
	for _, r := range rrs {
		if r.Header().Rrtype == dns.TypeRRSIG {
			n++
		}
	}
	return n
}

// Checks that a CNAME whose target lies outside every zone we serve is signed for the
// CNAME itself, while the out-of-zone target is left untouched.
func TestSigningCnameOutOfZone(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := &dns.Msg{
		Answer: []dns.RR{
			test.CNAME("www.miek.nl.\t1800\tIN\tCNAME\ttarget.example.com."),
			test.A("target.example.com.\t1800\tIN\tA\t127.0.0.1"),
		},
	}
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)

	if got := numSigs(m.Answer); got != 1 {
		t.Fatalf("Answer should have exactly 1 RRSIG (the in-zone CNAME), got %d", got)
	}
	for _, r := range m.Answer {
		sig, ok := r.(*dns.RRSIG)
		if !ok {
			continue
		}
		if sig.TypeCovered != dns.TypeCNAME {
			t.Errorf("RRSIG should cover CNAME, got %s", dns.TypeToString[sig.TypeCovered])
		}
		if sig.SignerName != "miek.nl." {
			t.Errorf("RRSIG signer should be miek.nl., got %s", sig.SignerName)
		}
	}
}

// checks that when a CNAME target lives in another zone we are also authoritative for,
// both RRsets are signed, each with the apex of the zone that actually contains its
// owner name as the signer.
func TestSigningCnameCrossZone(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl.", "example.org."})
	defer rm1()
	defer rm2()

	m := &dns.Msg{
		Answer: []dns.RR{
			test.CNAME("www.miek.nl.\t1800\tIN\tCNAME\tdb.example.org."),
			test.A("db.example.org.\t1800\tIN\tA\t127.0.0.1"),
		},
	}
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)

	if got := numSigs(m.Answer); got != 2 {
		t.Fatalf("Answer should have 2 RRSIGs (CNAME + cross-zone A), got %d", got)
	}
	for _, r := range m.Answer {
		sig, ok := r.(*dns.RRSIG)
		if !ok {
			continue
		}
		switch sig.TypeCovered {
		case dns.TypeCNAME:
			if sig.SignerName != "miek.nl." {
				t.Errorf("CNAME RRSIG signer should be miek.nl., got %s", sig.SignerName)
			}
		case dns.TypeA:
			if sig.SignerName != "example.org." {
				t.Errorf("cross-zone A RRSIG signer should be example.org., got %s", sig.SignerName)
			}
		default:
			t.Errorf("unexpected RRSIG covering %s", dns.TypeToString[sig.TypeCovered])
		}
	}
}
