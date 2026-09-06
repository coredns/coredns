package test

import (
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// TestDNS64FilterA exercises the `filter_a` dns64 option end-to-end against a
// real zone file (so the internal SOA lookup that filter_a issues for RFC 2308
// negative caching can resolve). It asserts:
//   - A queries return NOERROR/NODATA with the zone's SOA in the authority
//     section and EDE 17 "Filtered".
//   - AAAA queries are still synthesised from the zone's A records, with
//     EDE 29 "Synthesized".
func TestDNS64FilterA(t *testing.T) {
	zoneFile, rm, err := test.TempFile(".", `$ORIGIN example.
@	3600 IN	SOA ns.example. hostmaster.example. 1 7200 900 1209600 3600
	3600 IN NS ns.example.
v4only	60   IN A 192.0.2.42
ns	3600 IN A 192.0.2.1
`)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `.:0 {
		file ` + zoneFile + ` example
		dns64 {
			prefix 64:ff9b::/96
			translate_all
			allow_ipv4
			filter_a
		}
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// A query: NOERROR + empty answer + EDE 17.
	m := new(dns.Msg)
	m.SetQuestion("v4only.example.", dns.TypeA)
	m.SetEdns0(4096, false)

	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("A exchange failed: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("A query: want NOERROR, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("A query: want empty answer, got %v", resp.Answer)
	}
	// RFC 2308: NODATA should carry an SOA in the authority section so
	// downstream resolvers can negative-cache it. filter_a issues an internal
	// SOA query for the queried name; the `file` plugin answers with the zone
	// SOA in the Authority section (NODATA for SOA at a non-apex name).
	foundSOA := false
	for _, rr := range resp.Ns {
		if _, ok := rr.(*dns.SOA); ok {
			foundSOA = true
		}
	}
	if !foundSOA {
		t.Fatalf("A query: expected SOA in authority section for negative caching, got %v", resp.Ns)
	}
	opt := resp.IsEdns0()
	if opt == nil {
		t.Fatal("A query: expected OPT record carrying EDE, got none")
	}
	foundEDE := false
	for _, o := range opt.Option {
		if ede, ok := o.(*dns.EDNS0_EDE); ok && ede.InfoCode == dns.ExtendedErrorCodeFiltered {
			foundEDE = true
		}
	}
	if !foundEDE {
		t.Fatalf("A query: expected EDE code %d (Filtered), got %#v", dns.ExtendedErrorCodeFiltered, opt.Option)
	}

	// AAAA query: synthesised AAAA from the zone's A record, with EDE 29 Synthesized.
	m = new(dns.Msg)
	m.SetQuestion("v4only.example.", dns.TypeAAAA)
	m.SetEdns0(4096, false)
	resp, err = dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("AAAA exchange failed: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("AAAA query: want NOERROR, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) == 0 {
		t.Fatal("AAAA query: want synthesised AAAA, got empty — filter_a is shadowing dns64's own internal A lookup")
	}
	aaaa, ok := resp.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("AAAA query: want AAAA RR, got %T", resp.Answer[0])
	}
	want := net.ParseIP("64:ff9b::192.0.2.42")
	if !aaaa.AAAA.Equal(want) {
		t.Fatalf("AAAA query: want %s, got %s", want, aaaa.AAAA)
	}
	opt = resp.IsEdns0()
	if opt == nil {
		t.Fatal("AAAA query: expected OPT record carrying EDE 29 Synthesized, got none")
	}
	foundSyn := false
	for _, o := range opt.Option {
		if ede, ok := o.(*dns.EDNS0_EDE); ok && ede.InfoCode == dns.ExtendedErrorCodeSynthesized {
			foundSyn = true
		}
	}
	if !foundSyn {
		t.Fatalf("AAAA query: expected EDE code %d (Synthesized), got %#v", dns.ExtendedErrorCodeSynthesized, opt.Option)
	}
}
