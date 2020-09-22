package records

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"
)

func TestLookup(t *testing.T) {
	const input = `
records {
        example.org.   60  IN SOA ns.icann.org. noc.dns.icann.org. 2020091001 7200 3600 1209600 3600
        example.org.   60  IN MX 10 mx.example.org.
        mx.example.org. 60 IN A  127.0.0.1
}
`

	c := caddy.NewTestController("dns", input)
	re, err := recordsParse(c)
	if err != nil {
		t.Fatal(err)
	}

	for i, tc := range testCases {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := re.ServeDNS(context.Background(), rec, m)
		if err != nil {
			t.Errorf("Test %d, expected no error, got %v", i, err)
			return
		}

		if rec.Msg.Rcode != tc.Rcode {
			t.Errorf("Test %d, expected rcode is %d, but got %d", i, tc.Rcode, rec.Msg.Rcode)
			return
		}

		if resp := rec.Msg; rec.Msg != nil {
			if err := test.SortAndCheck(resp, tc); err != nil {
				t.Errorf("Test %d: %v", i, err)
			}
		}
	}
}

var testCases = []test.Case{
	{
		Qname: "mx.example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("mx.example.org. 60	IN	A 127.0.0.1"),
		},
	},
	{
		Rcode: dns.RcodeNameError,
		Qname: "bla.example.org.", Qtype: dns.TypeA,
		Ns: []dns.RR{
			test.SOA("example.org.   60  IN SOA ns.icann.org. noc.dns.icann.org. 2020091001 7200 3600 1209600 3600"),
		},
	},
	{
		Qname: "mx.example.org.", Qtype: dns.TypeAAAA,
		Ns: []dns.RR{
			test.SOA("example.org.   60  IN SOA ns.icann.org. noc.dns.icann.org. 2020091001 7200 3600 1209600 3600"),
		},
	},
}

func TestLookupNoSOA(t *testing.T) {
	const input = `
records {
        example.org.   60  IN MX 10 mx.example.org.
        mx.example.org. 60 IN A  127.0.0.1
}
`

	c := caddy.NewTestController("dns", input)
	re, err := recordsParse(c)
	if err != nil {
		t.Fatal(err)
	}

	for i, tc := range testCasesNoSOA {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := re.ServeDNS(context.Background(), rec, m)
		if err != nil {
			t.Errorf("Test %d, expected no error, got %v", i, err)
			return
		}

		if rec.Msg.Rcode != tc.Rcode {
			t.Errorf("Test %d, expected rcode is %d, but got %d", i, tc.Rcode, rec.Msg.Rcode)
			return
		}

		if resp := rec.Msg; rec.Msg != nil {
			if err := test.SortAndCheck(resp, tc); err != nil {
				t.Errorf("Test %d: %v", i, err)
			}
		}
	}
}

var testCasesNoSOA = []test.Case{
	{
		Qname: "mx.example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("mx.example.org. 60	IN	A 127.0.0.1"),
		},
	},
	{
		Rcode: dns.RcodeNameError,
		Qname: "bla.example.org.", Qtype: dns.TypeA,
	},
	{
		Qname: "mx.example.org.", Qtype: dns.TypeAAAA,
	},
}

func TestLookupMultipleOrigins(t *testing.T) {
	const input = `
records example.org example.net {
        @ 60  IN MX 10 mx
        mx 60 IN A  127.0.0.1
}
`

	c := caddy.NewTestController("dns", input)
	re, err := recordsParse(c)
	if err != nil {
		t.Fatal(err)
	}

	for i, tc := range testCasesMultipleOrigins {
		m := tc.Msg()

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := re.ServeDNS(context.Background(), rec, m)
		if err != nil {
			t.Errorf("Test %d, expected no error, got %v", i, err)
			return
		}

		if rec.Msg.Rcode != tc.Rcode {
			t.Errorf("Test %d, expected rcode is %d, but got %d", i, tc.Rcode, rec.Msg.Rcode)
			return
		}

		if resp := rec.Msg; rec.Msg != nil {
			if err := test.SortAndCheck(resp, tc); err != nil {
				t.Errorf("Test %d: %v", i, err)
			}
		}
	}
}

var testCasesMultipleOrigins = []test.Case{
	{
		Qname: "mx.example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("mx.example.org. 60	IN	A 127.0.0.1"),
		},
	},
	{
		Qname: "mx.example.net.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("mx.example.net. 60	IN	A 127.0.0.1"),
		},
	},
}
