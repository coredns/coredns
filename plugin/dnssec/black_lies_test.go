package dnssec

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestZoneSigningBlackLies(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testNxdomainMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)
	if !section(m.Ns, 2) {
		t.Errorf("Authority section should have 2 sigs")
	}
	var nsec *dns.NSEC
	for _, r := range m.Ns {
		if r.Header().Rrtype == dns.TypeNSEC {
			nsec = r.(*dns.NSEC)
		}
	}
	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected rcode %d, got %d", dns.RcodeSuccess, m.Rcode)
	}
	if nsec == nil {
		t.Fatalf("Expected NSEC, got none")
	}
	if nsec.Hdr.Name != "ww.miek.nl." {
		t.Errorf("Expected %s, got %s", "ww.miek.nl.", nsec.Hdr.Name)
	}
	if nsec.NextDomain != "\\000.ww.miek.nl." {
		t.Errorf("Expected %s, got %s", "\\000.ww.miek.nl.", nsec.NextDomain)
	}
}

func TestBlackLiesNoError(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testSuccessMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)

	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected rcode %d, got %d", dns.RcodeSuccess, m.Rcode)
	}

	if len(m.Answer) != 2 {
		t.Errorf("Answer section should have 2 RRs")
	}
	sig, txt := false, false
	for _, rr := range m.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			sig = true
		}
		if _, ok := rr.(*dns.TXT); ok {
			txt = true
		}
	}
	if !sig || !txt {
		t.Errorf("Expected RRSIG and TXT in answer section")
	}
}

func TestBlackLiesApexNsec(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testNsecMsg()
	m.SetQuestion("miek.nl.", dns.TypeNSEC)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)
	if len(m.Ns) > 0 {
		t.Error("Authority section should be empty")
	}
	if len(m.Answer) != 2 {
		t.Errorf("Answer section should have 2 RRs")
	}
	sig, nsec := false, false
	for _, rr := range m.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			sig = true
		}
		if rnsec, ok := rr.(*dns.NSEC); ok {
			nsec = true
			var bitpresent uint
			for _, typeBit := range rnsec.TypeBitMap {
				switch typeBit {
				case dns.TypeSOA:
					bitpresent |= 4
				case dns.TypeNSEC:
					bitpresent |= 1
				case dns.TypeRRSIG:
					bitpresent |= 2
				}
			}
			if bitpresent != 7 {
				t.Error("NSEC must have SOA, RRSIG and NSEC in its bitmap")
			}
		}
	}
	if !sig || !nsec {
		t.Errorf("Expected RRSIG and NSEC in answer section")
	}
}

func TestBlackLiesNsec(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testNsecMsg()
	m.SetQuestion("www.miek.nl.", dns.TypeNSEC)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)
	if len(m.Ns) > 0 {
		t.Error("Authority section should be empty")
	}
	if len(m.Answer) != 2 {
		t.Errorf("Answer section should have 2 RRs")
	}
	sig, nsec := false, false
	for _, rr := range m.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			sig = true
		}
		if rnsec, ok := rr.(*dns.NSEC); ok {
			nsec = true
			var bitpresent uint
			for _, typeBit := range rnsec.TypeBitMap {
				switch typeBit {
				case dns.TypeNSEC:
					bitpresent |= 1
				case dns.TypeRRSIG:
					bitpresent |= 2
				}
			}
			if bitpresent != 3 {
				t.Error("NSEC must have RRSIG and NSEC in its bitmap")
			}
		}
	}
	if !sig || !nsec {
		t.Errorf("Expected RRSIG and NSEC in answer section")
	}
}

func TestBlackLiesApexDS(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testApexDSMsg()
	m.SetQuestion("miek.nl.", dns.TypeDS)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)
	if !section(m.Ns, 2) {
		t.Errorf("Authority section should have 2 sigs")
	}
	var nsec *dns.NSEC
	for _, r := range m.Ns {
		if r.Header().Rrtype == dns.TypeNSEC {
			nsec = r.(*dns.NSEC)
		}
	}
	if nsec == nil {
		t.Error("Expected NSEC, got none")
	} else if correctNsecForDS(nsec) {
		t.Error("NSEC DS at the apex zone should cover all apex type.")
	}
}

func TestBlackLiesDS(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	m := testApexDSMsg()
	m.SetQuestion("sub.miek.nl.", dns.TypeDS)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)
	if !section(m.Ns, 2) {
		t.Errorf("Authority section should have 2 sigs")
	}
	var nsec *dns.NSEC
	for _, r := range m.Ns {
		if r.Header().Rrtype == dns.TypeNSEC {
			nsec = r.(*dns.NSEC)
		}
	}
	if nsec == nil {
		t.Error("Expected NSEC, got none")
	} else if !correctNsecForDS(nsec) {
		t.Error("NSEC DS should cover delegation type only.")
	}
}

func correctNsecForDS(nsec *dns.NSEC) bool {
	var bitmask uint
	/* Coherent TypeBitMap for NSEC of DS should contain at least:
	 * {TypeNS, TypeNSEC, TypeRRSIG} and no SOA.
	 * Any missing type will confuse resolver because
	 * it will prove that the dns query cannot be a delegation point,
	 * which will break trust resolution for unsigned delegated domain.
	 * No SOA is obvious for none apex query.
	 */
	for _, typeBitmask := range nsec.TypeBitMap {
		switch typeBitmask {
		case dns.TypeNS:
			bitmask |= 1
		case dns.TypeNSEC:
			bitmask |= 2
		case dns.TypeRRSIG:
			bitmask |= 4
		case dns.TypeSOA:
			return false
		}
	}
	return bitmask == 7
}

func testNxdomainMsg() *dns.Msg {
	return &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeNameError},
		Question: []dns.Question{{Name: "ww.miek.nl.", Qclass: dns.ClassINET, Qtype: dns.TypeTXT}},
		Ns:       []dns.RR{test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1461471181 14400 3600 604800 14400")},
	}
}

func testSuccessMsg() *dns.Msg {
	return &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeSuccess},
		Question: []dns.Question{{Name: "www.miek.nl.", Qclass: dns.ClassINET, Qtype: dns.TypeTXT}},
		Answer:   []dns.RR{test.TXT(`www.miek.nl.	1800	IN	TXT	"response"`)},
	}
}

func testNsecMsg() *dns.Msg {
	return &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeNameError},
		Question: []dns.Question{{Name: "www.miek.nl.", Qclass: dns.ClassINET, Qtype: dns.TypeNSEC}},
		Ns:       []dns.RR{test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1461471181 14400 3600 604800 14400")},
	}
}

func testApexDSMsg() *dns.Msg {
	return &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeNameError},
		Question: []dns.Question{{Name: "miek.nl.", Qclass: dns.ClassINET, Qtype: dns.TypeDS}},
		Ns:       []dns.RR{test.SOA("miek.nl.	1800	IN	SOA	linode.atoom.net. miek.miek.nl. 1461471181 14400 3600 604800 14400")},
	}
}
