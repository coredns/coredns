package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestLookupCache(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `example.org:0 {
		forward . ` + udp + `
		cache 10
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	t.Run("Long TTL", func(t *testing.T) {
		testCase(t, "example.org.", udp, 2, 10)
	})

	t.Run("Short TTL", func(t *testing.T) {
		testCase(t, "short.example.org.", udp, 1, 5)
	})

	t.Run("DNSSEC OPT", func(t *testing.T) {
		testCaseDNSSEC(t, "example.org.", udp, 4096)
	})

	t.Run("DNSSEC OPT", func(t *testing.T) {
		testCaseDNSSEC(t, "example.org.", udp, 0)
	})
}

func testCase(t *testing.T, name, addr string, expectAnsLen int, expectTTL uint32) {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	resp, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	if len(resp.Answer) != expectAnsLen {
		t.Fatalf("Expected %v RR in the answer section, got %v.", expectAnsLen, len(resp.Answer))
	}

	ttl := resp.Answer[0].Header().Ttl
	if ttl != expectTTL {
		t.Errorf("Expected TTL to be %d, got %d", expectTTL, ttl)
	}
}

func testCaseDNSSEC(t *testing.T, name, addr string, bufsize int) {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)

	if bufsize > 0 {
		o := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
		o.SetDo()
		o.SetUDPSize(uint16(bufsize))
		m.Extra = append(m.Extra, o)
	}
	resp, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	if len(resp.Extra) == 0 && bufsize == 0 {
		// no OPT, this is OK
		return
	}

	opt := resp.Extra[len(resp.Extra)-1]
	if x, ok := opt.(*dns.OPT); !ok && bufsize > 0 {
		t.Fatalf("Expected OPT RR, got %T", x)
	}
	if bufsize > 0 {
		if !opt.(*dns.OPT).Do() {
			t.Errorf("Expected DO bit to be set, got false")
		}
		if x := opt.(*dns.OPT).UDPSize(); int(x) != bufsize {
			t.Errorf("Expected %d bufsize, got %d", bufsize, x)
		}
	} else {
		if opt.Header().Rrtype == dns.TypeOPT {
			t.Errorf("Expected no OPT RR, but got one: %s", opt)
		}
	}
}

func TestLookupCacheWithoutEdns(t *testing.T) {
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `example.org:0 {
		forward . ` + udp + `
		cache 10
	}`

	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}
	if len(resp.Extra) == 0 {
		return
	}

	if resp.Extra[0].Header().Rrtype == dns.TypeOPT {
		t.Fatalf("Expected no OPT RR, but got: %s", resp.Extra[0])
	}
	t.Fatalf("Expected empty additional section, got %v", resp.Extra)
}
