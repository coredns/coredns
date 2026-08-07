// test/siit_test.go
package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
)

// TestSIITAfterForward is a regression test for the plugin.cfg ordering bug:
// siit must run AFTER forward in the plugin chain so it can see (and rewrite)
// the AAAA answer forward returns, rather than running before it and never
// seeing the final response.
//
// Unlike the unit tests in plugin/siit, which instantiate SIIT directly and
// wire d.Next/d.Upstream to fakes, this test drives the real plugin chain
// built from plugin.cfg via a Corefile, so a future ordering regression
// (siit placed after cache/forward again) will fail here even though it
// can't be detected by the package-level unit tests.
func TestSIITAfterForward(t *testing.T) {
	// Upstream "authoritative" server: NODATA for A, a mapped AAAA for AAAA.
	upstream := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)

		switch r.Question[0].Qtype {
		case dns.TypeA:
			// NODATA: success, no answer section -- this is what makes
			// siit's responseShouldSIIT return true.
		case dns.TypeAAAA:
			rr, err := dns.NewRR("example.org. 60 IN AAAA 64:ff9b::192.0.2.42")
			if err != nil {
				t.Fatalf("failed to build AAAA record: %v", err)
			}
			m.Answer = []dns.RR{rr}
		}

		w.WriteMsg(m)
	})
	defer upstream.Close()

	corefile := `example.org:0 {
		siit {
			ipv6_prefix 64:ff9b::/96
		}
		forward . ` + upstream.Addr + `
	}`

	server, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("could not start CoreDNS test server: %v", err)
	}
	defer server.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)

	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected RcodeSuccess, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer record, got %d: %v", len(resp.Answer), resp.Answer)
	}

	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected an A record in the answer, got %T: %v", resp.Answer[0], resp.Answer[0])
	}

	want := "192.0.2.42"
	if a.A.String() != want {
		t.Errorf("expected synthesized A record %s, got %s -- if this is empty/unset, siit likely ran before forward in the plugin chain and never saw the upstream's AAAA answer", want, a.A.String())
	}
}
