package oneaddr

import (
	"context"
	"net"
	"sort"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestOneAddrInvocation(t *testing.T) {
	x := OneAddr{Next: test.ErrorHandler()}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	x.ServeDNS(ctx, rec, r)
	if rec.Rcode != dns.RcodeServerFailure {
		t.Errorf("unexpected Rcode in the response recorder: %d while expected %d", rec.Rcode, dns.RcodeServerFailure)
	}
	if rec.Msg == nil {
		t.Fatal("unexpected nil Msg in the response recorder")
	}
}

func TestOneAddrSimple(t *testing.T) {
	x := OneAddr{
		Next: test.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Authoritative = true
			m.Answer = []dns.RR{
				test.A("example.org.		300	IN	A			10.240.0.1"),
				test.A("example.org.		300	IN	A			10.240.0.2"),
			}
			w.WriteMsg(m)
			return dns.RcodeSuccess, nil
		}),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	x.ServeDNS(ctx, rec, r)
	if rec.Rcode != dns.RcodeSuccess {
		t.Errorf("unexpected Rcode in the response recorder: %d while expected %d", rec.Rcode, dns.RcodeSuccess)
	}
	if rec.Msg == nil {
		t.Fatal("unexpected nil Msg in the response recorder")
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("unexpected length of Msg.Answer: %d", len(rec.Msg.Answer))
	}
	if rec.Msg.Answer[0].Header().Rrtype != dns.TypeA {
		t.Fatalf("Expected A record for first answer, got %s", dns.TypeToString[rec.Msg.Answer[0].Header().Rrtype])
	}
	a, ok := rec.Msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("type assertion failed: %T != *dns.A", rec.Msg.Answer[0])
	}
	if !a.A.Equal(net.IPv4(10, 240, 0, 1)) {
		t.Fatalf("unexpected address in the answer record: %s", a.A.String())
	}
}

func TestOneAddrComplex(t *testing.T) {
	x := OneAddr{
		Next: test.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Authoritative = true
			m.Answer = []dns.RR{
				test.CNAME("www2.example.org.	300	IN	CNAME		www.example.org."),
				test.A("www.example.org.		300	IN	A			10.240.0.1"),
				test.A("www.example.org.		300	IN	A			10.240.0.2"),
				test.A("www.example.org.		300	IN	A			10.240.0.3"),
				test.AAAA("www.example.org.		300	IN	AAAA		::1"),
				test.AAAA("www.example.org.		300	IN	AAAA		::2"),
				test.AAAA("www.example.org.		300	IN	AAAA		::3"),
			}
			w.WriteMsg(m)
			return dns.RcodeSuccess, nil
		}),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("www2.example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	x.ServeDNS(ctx, rec, r)
	if rec.Rcode != dns.RcodeSuccess {
		t.Errorf("unexpected Rcode in the response recorder: %d while expected %d", rec.Rcode, dns.RcodeSuccess)
	}
	if rec.Msg == nil {
		t.Fatal("unexpected nil Msg in the response recorder")
	}
	tc := test.Case{
		Qname: "www2.example.org.",
		Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("www2.example.org.	300	IN	CNAME		www.example.org."),
			test.A("www.example.org.		300	IN	A			10.240.0.1"),
			test.AAAA("www.example.org.		300	IN	AAAA		::1"),
		},
		Ns:    []dns.RR{},
		Extra: []dns.RR{},
	}
	sort.Sort(test.RRSet(tc.Answer))
	if err := test.SortAndCheck(rec.Msg, tc); err != nil {
		t.Log(rec.Msg.String())
		t.Errorf("sort and check failed: %v", err)
	}
}
