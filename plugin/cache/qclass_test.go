package cache

import (
	"context"
	"slices"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestCacheSeparatesQuestionClasses(t *testing.T) {
	const qname = "victim.cache-class.test."

	classes := []uint16{}
	c := New()
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		qclass := r.Question[0].Qclass
		classes = append(classes, qclass)

		m := new(dns.Msg)
		m.SetReply(r)
		if qclass == dns.ClassCHAOS {
			m.Rcode = dns.RcodeNameError
			m.Ns = []dns.RR{test.SOA("cache-class.test. 60 CH SOA ns.cache-class.test. hostmaster.cache-class.test. 1 60 60 60 60")}
		} else {
			m.Answer = []dns.RR{test.A(qname + " 60 IN A 192.0.2.1")}
		}

		if err := w.WriteMsg(m); err != nil {
			return dns.RcodeServerFailure, err
		}
		return m.Rcode, nil
	})

	query := func(qclass uint16) *dns.Msg {
		t.Helper()
		req := new(dns.Msg)
		req.SetQuestion(qname, dns.TypeA)
		req.Question[0].Qclass = qclass
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := c.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("ServeDNS for class %d failed: %v", qclass, err)
		}
		return rec.Msg
	}

	if got := query(dns.ClassCHAOS).Rcode; got != dns.RcodeNameError {
		t.Fatalf("CHAOS query rcode = %s, want NXDOMAIN", dns.RcodeToString[got])
	}

	inet := query(dns.ClassINET)
	if inet.Rcode != dns.RcodeSuccess || len(inet.Answer) != 1 {
		t.Fatalf("IN query returned rcode %s with %d answers, want NOERROR with one answer", dns.RcodeToString[inet.Rcode], len(inet.Answer))
	}

	if want := []uint16{dns.ClassCHAOS, dns.ClassINET}; !slices.Equal(classes, want) {
		t.Fatalf("upstream query classes = %v, want %v", classes, want)
	}
}
