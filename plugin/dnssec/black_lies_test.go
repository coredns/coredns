package dnssec

import (
	"crypto"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/cache"
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

type failAfterNSigner struct {
	inner crypto.Signer
	allow int32
	calls atomic.Int32
}

func (f *failAfterNSigner) Public() crypto.PublicKey { return f.inner.Public() }

func (f *failAfterNSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if f.calls.Add(1) > f.allow {
		return nil, errors.New("simulated signing failure")
	}
	return f.inner.Sign(rand, digest, opts)
}

func TestBlackLiesNsecMultiKey(t *testing.T) {
	d, rm1, rm2 := newDnssec(t, []string{"miek.nl."})
	defer rm1()
	defer rm2()

	fPriv1, rmPriv1, err := test.TempFile(".", privKey1)
	if err != nil {
		t.Fatalf("Failed to create temp private key: %v", err)
	}
	fPub1, rmPub1, err := test.TempFile(".", pubKey1)
	if err != nil {
		rmPriv1()
		t.Fatalf("Failed to create temp public key: %v", err)
	}
	defer rmPriv1()
	defer rmPub1()

	key1, err := ParseKeyFile(fPub1, fPriv1)
	if err != nil {
		t.Fatalf("Failed to parse key: %v", err)
	}
	d.keys = append(d.keys, key1)

	m := testNsecMsg()
	m.SetQuestion("www.miek.nl.", dns.TypeNSEC)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)

	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected rcode %d, got %d", dns.RcodeSuccess, m.Rcode)
	}
	if len(m.Ns) != 0 {
		t.Errorf("Authority section should be empty, got %d RRs", len(m.Ns))
	}
	if len(m.Answer) != 3 {
		t.Fatalf("Answer section should have 3 RRs (2 RRSIGs + 1 NSEC), got %d", len(m.Answer))
	}
	sigs := make(map[uint16]bool)
	nsecs := 0
	for _, rr := range m.Answer {
		switch r := rr.(type) {
		case *dns.RRSIG:
			if r.TypeCovered != dns.TypeNSEC {
				t.Errorf("Expected RRSIG covering NSEC, got %s", dns.TypeToString[r.TypeCovered])
			}
			sigs[r.KeyTag] = true
		case *dns.NSEC:
			nsecs++
		default:
			t.Errorf("Unexpected RR type in Answer: %s", dns.TypeToString[rr.Header().Rrtype])
		}
	}
	if len(sigs) != 2 || nsecs != 1 {
		t.Errorf("Expected 2 distinct RRSIGs and 1 NSEC in Answer, got %d sigs and %d NSECs", len(sigs), nsecs)
	}
}

func TestBlackLiesNsecSplitKeysKSKOnly(t *testing.T) {
	k, rm1, rm2 := newKey(t)
	defer rm1()
	defer rm2()

	d := New([]string{"miek.nl."}, []*DNSKEY{k}, true, nil, cache.New[[]dns.RR](defaultCap))

	m := testNsecMsg()
	m.SetQuestion("www.miek.nl.", dns.TypeNSEC)
	state := request.Request{Req: m, Zone: "miek.nl."}
	m = d.Sign(state, time.Now().UTC(), server)

	if len(m.Ns) != 0 {
		t.Errorf("Authority section should be empty, got %d RRs", len(m.Ns))
	}
	if len(m.Answer) != 1 {
		t.Fatalf("Answer section should have 1 NSEC RR, got %d: %v", len(m.Answer), m.Answer)
	}
	if _, ok := m.Answer[0].(*dns.NSEC); !ok {
		t.Errorf("Expected NSEC in Answer, got %T", m.Answer[0])
	}
}

func TestBlackLiesNsecSigningFailure(t *testing.T) {
	k, rm1, rm2 := newKey(t)
	defer rm1()
	defer rm2()

	for _, tc := range []struct {
		name  string
		build func() *dns.Msg
	}{
		{name: "nxdomain", build: testNxdomainMsg},
		{name: "nsec_query", build: testNsecMsg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flakyKey := &DNSKEY{
				K:   dns.Copy(k.K).(*dns.DNSKEY),
				s:   &failAfterNSigner{inner: k.s, allow: 1}, // SOA succeeds, NSEC fails
				tag: k.tag,
			}
			d := New([]string{"miek.nl."}, []*DNSKEY{flakyKey}, false, nil, cache.New[[]dns.RR](defaultCap))

			m := tc.build()
			state := request.Request{Req: m, Zone: "miek.nl."}
			m = d.Sign(state, time.Now().UTC(), server)

			if m.Rcode != dns.RcodeNameError {
				t.Errorf("Expected RcodeNameError when NSEC signing fails, got %d", m.Rcode)
			}
			if len(m.Answer) != 0 {
				t.Errorf("Expected empty Answer when NSEC signing fails, got %v", m.Answer)
			}
			if len(m.Ns) != 2 {
				t.Errorf("Expected SOA + RRSIG(SOA) in Ns, got %d RRs", len(m.Ns))
			}
		})
	}
}

func TestBlackLiesNsecCachedSigsNoAliasing(t *testing.T) {
	k, rm1, rm2 := newKey(t)
	defer rm1()
	defer rm2()

	// Three keys make d.sign() cache a slice with len=3, cap=4; appending nsec
	// directly to that slice would race across concurrent callers.
	k2 := &DNSKEY{K: dns.Copy(k.K).(*dns.DNSKEY), s: k.s, tag: k.tag + 1}
	k3 := &DNSKEY{K: dns.Copy(k.K).(*dns.DNSKEY), s: k.s, tag: k.tag + 2}
	d := New([]string{"miek.nl."}, []*DNSKEY{k, k2, k3}, false, nil, cache.New[[]dns.RR](defaultCap))

	now := time.Now().UTC()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 10 {
				m := testNsecMsg()
				m.SetQuestion("www.miek.nl.", dns.TypeNSEC)
				res := d.Sign(request.Request{Req: m, Zone: "miek.nl."}, now, server)
				if len(res.Answer) != 4 {
					t.Errorf("Expected 4 RRs (3 RRSIGs + 1 NSEC), got %d", len(res.Answer))
				}
			}
		})
	}
	wg.Wait()
}
