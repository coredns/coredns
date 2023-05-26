package local

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var testcases = []struct {
	question string
	qtype    uint16
	rcode    int
	answer   dns.RR
	ns       dns.RR
}{
	{"localhost.", dns.TypeA, dns.RcodeSuccess, test.A("localhost. IN A 127.0.0.1"), nil},
	{"localHOst.", dns.TypeA, dns.RcodeSuccess, test.A("localHOst. IN A 127.0.0.1"), nil},
	{"localhost.", dns.TypeAAAA, dns.RcodeSuccess, test.AAAA("localhost. IN AAAA ::1"), nil},
	{"localhost.", dns.TypeNS, dns.RcodeSuccess, test.NS("localhost. IN NS localhost."), nil},
	{"localhost.", dns.TypeSOA, dns.RcodeSuccess, test.SOA("localhost. IN SOA root.localhost. localhost. 1 0 0 0 0"), nil},
	{"127.in-addr.arpa.", dns.TypeA, dns.RcodeSuccess, nil, test.SOA("127.in-addr.arpa. IN SOA root.localhost. localhost. 1 0 0 0 0")},
	{"localhost.", dns.TypeMX, dns.RcodeSuccess, nil, test.SOA("localhost. IN SOA root.localhost. localhost. 1 0 0 0 0")},
	{"a.localhost.", dns.TypeA, dns.RcodeNameError, nil, test.SOA("localhost. IN SOA root.localhost. localhost. 1 0 0 0 0")},
	{"1.0.0.127.in-addr.arpa.", dns.TypePTR, dns.RcodeSuccess, test.PTR("1.0.0.127.in-addr.arpa. IN PTR localhost."), nil},
	{"1.0.0.127.in-addr.arpa.", dns.TypeMX, dns.RcodeSuccess, nil, test.SOA("127.in-addr.arpa. IN SOA root.localhost. localhost. 1 0 0 0 0")},
	{"2.0.0.127.in-addr.arpa.", dns.TypePTR, dns.RcodeNameError, nil, test.SOA("127.in-addr.arpa. IN SOA root.localhost. localhost. 1 0 0 0 0")},
	{"localhost.example.net.", dns.TypeA, dns.RcodeSuccess, test.A("localhost.example.net. IN A 127.0.0.1"), nil},
	{"localhost.example.net.", dns.TypeAAAA, dns.RcodeSuccess, test.AAAA("localhost.example.net IN AAAA ::1"), nil},
	{"localhost.example.net.", dns.TypeSOA, dns.RcodeSuccess, nil, test.SOA("localhost.example.net. IN SOA root.localhost.example.net. localhost.example.net. 1 0 0 0 0")},
}

func TestLocal(t *testing.T) {
	req := new(dns.Msg)
	l := &Local{}

	for i, tc := range testcases {
		req.SetQuestion(tc.question, tc.qtype)
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := l.ServeDNS(context.TODO(), rec, req)

		if err != nil {
			t.Errorf("Test %d, expected no error, but got %q", i, err)
			continue
		}
		if rec.Msg.Rcode != tc.rcode {
			t.Errorf("Test %d, expected rcode %d, got %d", i, tc.rcode, rec.Msg.Rcode)
		}
		if tc.answer == nil && len(rec.Msg.Answer) > 0 {
			t.Errorf("Test %d, expected no answer RR, got %s", i, rec.Msg.Answer[0])
			continue
		}
		if tc.ns == nil && len(rec.Msg.Ns) > 0 {
			t.Errorf("Test %d, expected no authority RR, got %s", i, rec.Msg.Ns[0])
			continue
		}
		if tc.answer != nil {
			if x := tc.answer.Header().Rrtype; x != rec.Msg.Answer[0].Header().Rrtype {
				t.Errorf("Test %d, expected RR type %d in answer, got %d", i, x, rec.Msg.Answer[0].Header().Rrtype)
			}
			if x := tc.answer.Header().Name; x != rec.Msg.Answer[0].Header().Name {
				t.Errorf("Test %d, expected RR name %q in answer, got %q", i, x, rec.Msg.Answer[0].Header().Name)
			}
		}
		if tc.ns != nil {
			if x := tc.ns.Header().Rrtype; x != rec.Msg.Ns[0].Header().Rrtype {
				t.Errorf("Test %d, expected RR type %d in authority, got %d", i, x, rec.Msg.Ns[0].Header().Rrtype)
			}
			if x := tc.ns.Header().Name; x != rec.Msg.Ns[0].Header().Name {
				t.Errorf("Test %d, expected RR name %q in authority, got %q", i, x, rec.Msg.Ns[0].Header().Name)
			}
		}
	}
}
