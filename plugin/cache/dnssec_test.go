package cache

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestResponseWithDNSSEC(t *testing.T) {
	// We do 2 queries, one where we want non-dnssec and one with dnssec and check the responses in each of them
	var tcs = []test.Case{
		{
			Qname: "invent.example.org.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("invent.example.org.		1781	IN	CNAME	leptone.example.org."),
				test.A("leptone.example.org.	1781	IN	A	195.201.182.103"),
			},
		},
		{
			Qname: "invent.example.org.", Qtype: dns.TypeA,
			Do: true,
			Answer: []dns.RR{
				test.CNAME("invent.example.org.		1781	IN	CNAME	leptone.example.org."),
				test.RRSIG("invent.example.org.		1781	IN	RRSIG	CNAME 8 3 1800 20201012085750 20200912082613 57411 example.org. ijSv5FmsNjFviBcOFwQgqjt073lttxTTNqkno6oMa3DD3kC+"),
				test.A("leptone.example.org.	1781	IN	A	195.201.182.103"),
				test.RRSIG("leptone.example.org.	1781	IN	RRSIG	A 8 3 1800 20201012093630 20200912083827 57411 example.org. eLuSOkLAzm/WIOpaZD3/4TfvKP1HAFzjkis9LIJSRVpQt307dm9WY9"),
			},
		},
	}

	c := New()
	c.Next = dnssecHandler()

	for i, tc := range tcs {
		m := tc.Msg()
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, m)
		if err := test.Section(tc, test.Answer, rec.Msg.Answer); err != nil {
			t.Errorf("Test %d, expected no error, got %s", i, err)
		}
	}

	// now do the reverse
	c = New()
	c.Next = dnssecHandler()

	for i, tc := range []test.Case{tcs[1], tcs[0]} {
		m := tc.Msg()
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.ServeDNS(context.TODO(), rec, m)
		if err := test.Section(tc, test.Answer, rec.Msg.Answer); err != nil {
			t.Errorf("Test %d, expected no error, got %s", i, err)
		}
	}
}

func dnssecHandler() plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetQuestion("example.org.", dns.TypeA)

		m.Answer = make([]dns.RR, 4)
		m.Answer[0] = test.CNAME("invent.example.org.		1781	IN	CNAME	leptone.example.org.")
		m.Answer[1] = test.RRSIG("invent.example.org.		1781	IN	RRSIG	CNAME 8 3 1800 20201012085750 20200912082613 57411 example.org. ijSv5FmsNjFviBcOFwQgqjt073lttxTTNqkno6oMa3DD3kC+")
		m.Answer[2] = test.A("leptone.example.org.	1781	IN	A	195.201.182.103")
		m.Answer[3] = test.RRSIG("leptone.example.org.	1781	IN	RRSIG	A 8 3 1800 20201012093630 20200912083827 57411 example.org. eLuSOkLAzm/WIOpaZD3/4TfvKP1HAFzjkis9LIJSRVpQt307dm9WY9")
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}

func TestFliterRRSlice(t *testing.T) {
	rrs := []dns.RR{
		test.CNAME("invent.example.org.		1781	IN	CNAME	leptone.example.org."),
		test.RRSIG("invent.example.org.		1781	IN	RRSIG	CNAME 8 3 1800 20201012085750 20200912082613 57411 example.org. ijSv5FmsNjFviBcOFwQgqjt073lttxTTNqkno6oMa3DD3kC+"),
		test.A("leptone.example.org.	1781	IN	A	195.201.182.103"),
		test.RRSIG("leptone.example.org.	1781	IN	RRSIG	A 8 3 1800 20201012093630 20200912083827 57411 example.org. eLuSOkLAzm/WIOpaZD3/4TfvKP1HAFzjkis9LIJSRVpQt307dm9WY9"),
	}

	filter1 := filterRRSlice(rrs, 0, true, false)
	if len(filter1) != 4 {
		t.Errorf("Expected 4 RRs after filtering, got %d", len(filter1))
	}
	rrsig := 0
	for _, f := range filter1 {
		if f.Header().Rrtype == dns.TypeRRSIG {
			rrsig++
		}
	}
	if rrsig != 2 {
		t.Errorf("Expected 2 RRSIGs after filtering, got %d", rrsig)
	}

	filter2 := filterRRSlice(rrs, 0, false, false)
	if len(filter2) != 2 {
		t.Errorf("Expected 2 RRs after filtering, got %d", len(filter2))
	}
	rrsig = 0
	for _, f := range filter2 {
		if f.Header().Rrtype == dns.TypeRRSIG {
			rrsig++
		}
	}
	if rrsig != 0 {
		t.Errorf("Expected 0 RRSIGs after filtering, got %d", rrsig)
	}
}
