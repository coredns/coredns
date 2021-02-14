package rewrite

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var tests = []struct {
	from     string
	fromType uint16
	answer   []dns.RR
	to       string
	toType   uint16
	noRevert bool
}{
	{"core.dns.rocks", dns.TypeA, []dns.RR{test.A("dns.core.rocks.  5   IN  A  10.0.0.1")}, "core.dns.rocks", dns.TypeA, false},
	{"core.dns.rocks", dns.TypeSRV, []dns.RR{test.SRV("dns.core.rocks.  5  IN  SRV 0 100 100 srv1.dns.core.rocks.")}, "core.dns.rocks", dns.TypeSRV, false},
	{"core.dns.rocks", dns.TypeA, []dns.RR{test.A("core.dns.rocks.  5   IN  A  10.0.0.1")}, "dns.core.rocks.", dns.TypeA, true},
	{"core.dns.rocks", dns.TypeSRV, []dns.RR{test.SRV("core.dns.rocks.  5  IN  SRV 0 100 100 srv1.dns.core.rocks.")}, "dns.core.rocks.", dns.TypeSRV, true},
	{"core.dns.rocks", dns.TypeHINFO, []dns.RR{test.HINFO("core.dns.rocks.  5  HINFO INTEL-64 \"RHEL 7.4\"")}, "core.dns.rocks", dns.TypeHINFO, false},
	{"core.dns.rocks", dns.TypeA, []dns.RR{
		test.A("dns.core.rocks.  5   IN  A  10.0.0.1"),
		test.A("dns.core.rocks.  5   IN  A  10.0.0.2"),
	}, "core.dns.rocks", dns.TypeA, false},
}

func TestResponseReverter(t *testing.T) {

	rules := []Rule{}
	r, _ := newNameRule("stop", "regex", `(core)\.(dns)\.(rocks)`, "{2}.{1}.{3}", "answer", "name", `(dns)\.(core)\.(rocks)`, "{2}.{1}.{3}")
	rules = append(rules, r)

	doReverterTests(rules, t)

	rules = []Rule{}
	r, _ = newNameRule("continue", "regex", `(core)\.(dns)\.(rocks)`, "{2}.{1}.{3}", "answer", "name", `(dns)\.(core)\.(rocks)`, "{2}.{1}.{3}")
	rules = append(rules, r)

	doReverterTests(rules, t)
}

func doReverterTests(rules []Rule, t *testing.T) {
	ctx := context.TODO()
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.from, tc.fromType)
		m.Question[0].Qclass = dns.ClassINET
		m.Answer = tc.answer
		rw := Rewrite{
			Next:     plugin.HandlerFunc(msgPrinter),
			Rules:    rules,
			noRevert: tc.noRevert,
		}
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)
		resp := rec.Msg
		if resp.Question[0].Name != tc.to {
			t.Errorf("Test %d: Expected Name to be %q but was %q", i, tc.to, resp.Question[0].Name)
		}
		if resp.Question[0].Qtype != tc.toType {
			t.Errorf("Test %d: Expected Type to be '%d' but was '%d'", i, tc.toType, resp.Question[0].Qtype)
		}
	}
}

var valueTests = []struct {
	from             string
	fromType         uint16
	answer           []dns.RR
	extra            []dns.RR
	to               string
	toType           uint16
	noRevert         bool
	expectValue     string
	expectAnswerType uint16
	expectAddlName   string
}{
	{"my.domain.uk", dns.TypeSRV, []dns.RR{test.SRV("my.cluster.local.  5  IN  SRV 0 100 100 srv1.my.cluster.local.")}, []dns.RR{test.A("srv1.my.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeSRV, false, "srv1.my.domain.uk.", dns.TypeSRV, "srv1.my.domain.uk."},
	{"my.domain.uk", dns.TypeSRV, []dns.RR{test.SRV("my.cluster.local.  5  IN  SRV 0 100 100 srv1.my.cluster.local.")}, []dns.RR{test.A("srv1.my.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeSRV, true, "srv1.my.cluster.local.", dns.TypeSRV, "srv1.my.cluster.local."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.CNAME("my.cluster.local.  3600 IN CNAME cname.cluster.local.")}, []dns.RR{test.A("cname.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeANY, false, "cname.domain.uk.", dns.TypeCNAME, "cname.domain.uk."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.CNAME("my.cluster.local.  3600 IN CNAME cname.cluster.local.")}, []dns.RR{test.A("cname.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeANY, true, "cname.cluster.local.", dns.TypeCNAME, "cname.cluster.local."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.DNAME("my.cluster.local.  3600 IN DNAME dname.cluster.local.")}, []dns.RR{test.A("dname.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeANY, false, "dname.domain.uk.", dns.TypeDNAME, "dname.domain.uk."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.DNAME("my.cluster.local.  3600 IN DNAME dname.cluster.local.")}, []dns.RR{test.A("dname.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeANY, true, "dname.cluster.local.", dns.TypeDNAME, "dname.cluster.local."},
	{"my.domain.uk", dns.TypeMX, []dns.RR{test.MX("my.cluster.local.	3600	IN	MX	1 mx1.cluster.local.")}, []dns.RR{test.A("mx1.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeMX, false, "mx1.domain.uk.", dns.TypeMX, "mx1.domain.uk."},
	{"my.domain.uk", dns.TypeMX, []dns.RR{test.MX("my.cluster.local.	3600	IN	MX	1 mx1.cluster.local.")}, []dns.RR{test.A("mx1.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeMX, true, "mx1.cluster.local.", dns.TypeMX, "mx1.cluster.local."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.NS("my.cluster.local.	3600	IN	NS	ns1.cluster.local.")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeANY, false, "ns1.domain.uk.", dns.TypeNS, "ns1.domain.uk."},
	{"my.domain.uk", dns.TypeANY, []dns.RR{test.NS("my.cluster.local.	3600	IN	NS	ns1.cluster.local.")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeANY, true, "ns1.cluster.local.", dns.TypeNS, "ns1.cluster.local."},
	{"my.domain.uk", dns.TypeSOA, []dns.RR{test.SOA("my.cluster.local.		1800	IN	SOA	ns1.cluster.local. admin.cluster.local. 1502165581 14400 3600 604800 14400")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeSOA, false, "ns1.domain.uk.", dns.TypeSOA, "ns1.domain.uk."},
	{"my.domain.uk", dns.TypeSOA, []dns.RR{test.SOA("my.cluster.local.		1800	IN	SOA	ns1.cluster.local. admin.cluster.local. 1502165581 14400 3600 604800 14400")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeSOA, true, "ns1.cluster.local.", dns.TypeSOA, "ns1.cluster.local."},
	{"my.domain.uk", dns.TypeNAPTR, []dns.RR{test.NAPTR("my.cluster.local.  100  IN NAPTR 100 10 \"S\" \"SIP+D2U\" \"!^.*$!sip:customer-service@example.com!\" _sip._udp.cluster.local.")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.domain.uk", dns.TypeNAPTR, false, "_sip._udp.domain.uk.", dns.TypeNAPTR, "ns1.domain.uk."},
	{"my.domain.uk", dns.TypeNAPTR, []dns.RR{test.NAPTR("my.cluster.local.  100  IN NAPTR 100 10 \"S\" \"SIP+D2U\" \"!^.*$!sip:customer-service@example.com!\" _sip._udp.cluster.local.")}, []dns.RR{test.A("ns1.cluster.local.  5   IN  A  10.0.0.1")}, "my.cluster.local.", dns.TypeNAPTR, true, "_sip._udp.cluster.local.", dns.TypeNAPTR, "ns1.cluster.local."},
}

func TestValueResponseReverter(t *testing.T) {

	rules := []Rule{}
	r, _ := newNameRule("stop", "regex", `(.*)\.domain\.uk`, "{1}.cluster.local", "answer", "name", `(.*)\.cluster\.local`, "{1}.domain.uk", "answer", "value", `(.*)\.cluster\.local`, "{1}.domain.uk")
	rules = append(rules, r)

	doValueReverterTests(rules, t)

	rules = []Rule{}
	r, _ = newNameRule("continue", "regex", `(.*)\.domain\.uk`, "{1}.cluster.local", "answer", "name", `(.*)\.cluster\.local`, "{1}.domain.uk", "answer", "value", `(.*)\.cluster\.local`, "{1}.domain.uk")
	rules = append(rules, r)

	doValueReverterTests(rules, t)
}

func doValueReverterTests(rules []Rule, t *testing.T) {
	ctx := context.TODO()
	for i, tc := range valueTests {
		m := new(dns.Msg)
		m.SetQuestion(tc.from, tc.fromType)
		m.Question[0].Qclass = dns.ClassINET
		m.Answer = tc.answer
		m.Extra = tc.extra
		rw := Rewrite{
			Next:     plugin.HandlerFunc(msgPrinter),
			Rules:    rules,
			noRevert: tc.noRevert,
		}
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)
		resp := rec.Msg
		if resp.Question[0].Name != tc.to {
			t.Errorf("Test %d: Expected Name to be %q but was %q", i, tc.to, resp.Question[0].Name)
		}
		if resp.Question[0].Qtype != tc.toType {
			t.Errorf("Test %d: Expected Type to be '%d' but was '%d'", i, tc.toType, resp.Question[0].Qtype)
		}

		if len(resp.Answer) <= 0 || resp.Answer[0].Header().Rrtype != tc.expectAnswerType {
			t.Error("Unexpected Answer Record Type / No Answers")
			return
		}

		value := getRecordValueForRewrite(resp.Answer[0])
		if value != tc.expectValue {
			t.Errorf("Test %d: Expected Target to be '%s' but was '%s'", i, tc.expectValue, value)
		}

		if len(resp.Extra) <= 0 || resp.Extra[0].Header().Rrtype != dns.TypeA {
			t.Error("Unexpected Additional Record Type / No Additional Records")
			return
		}

		if resp.Extra[0].Header().Name != tc.expectAddlName {
			t.Errorf("Test %d: Expected Extra Name to be %q but was %q", i, tc.expectAddlName, resp.Extra[0].Header().Name)
		}
	}
}
