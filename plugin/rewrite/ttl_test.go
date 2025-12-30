package rewrite

import (
	"context"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestNewTTLRule(t *testing.T) {
	tests := []struct {
		next         string
		args         []string
		expectedFail bool
	}{
		{"stop", []string{"srv1.coredns.rocks", "10"}, false},
		{"stop", []string{"exact", "srv1.coredns.rocks", "15"}, false},
		{"stop", []string{"prefix", "coredns.rocks", "20"}, false},
		{"stop", []string{"suffix", "srv1", "25"}, false},
		{"stop", []string{"substring", "coredns", "30"}, false},
		{"stop", []string{"regex", `(srv1)\.(coredns)\.(rocks)`, "35"}, false},
		{"continue", []string{"srv1.coredns.rocks", "10"}, false},
		{"continue", []string{"exact", "srv1.coredns.rocks", "15"}, false},
		{"continue", []string{"prefix", "coredns.rocks", "20"}, false},
		{"continue", []string{"suffix", "srv1", "25"}, false},
		{"continue", []string{"substring", "coredns", "30"}, false},
		{"continue", []string{"regex", `(srv1)\.(coredns)\.(rocks)`, "35"}, false},
		{"stop", []string{"srv1.coredns.rocks", "12345678901234567890"}, true},
		{"stop", []string{"srv1.coredns.rocks", "coredns.rocks"}, true},
		{"stop", []string{"srv1.coredns.rocks", "#1"}, true},
		{"stop", []string{"range.coredns.rocks", "1-2"}, false},
		{"stop", []string{"ceil.coredns.rocks", "-2"}, false},
		{"stop", []string{"floor.coredns.rocks", "1-"}, false},
		{"stop", []string{"range.coredns.rocks", "2-2"}, false},
		{"stop", []string{"invalid.coredns.rocks", "-"}, true},
		{"stop", []string{"invalid.coredns.rocks", "2-1"}, true},
		{"stop", []string{"invalid.coredns.rocks", "5-10-20"}, true},
	}
	for i, tc := range tests {
		failed := false
		rule, err := newTTLRule(tc.next, tc.args...)
		if err != nil {
			failed = true
		}
		if !failed && !tc.expectedFail {
			continue
		}
		if failed && tc.expectedFail {
			continue
		}
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule)
	}
	for i, tc := range tests {
		failed := false
		tc.args = append([]string{tc.next, "ttl"}, tc.args...)
		rule, err := newRule(tc.args...)
		if err != nil {
			failed = true
		}
		if !failed && !tc.expectedFail {
			continue
		}
		if failed && tc.expectedFail {
			continue
		}
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule)
	}
}

func TestTtlRewrite(t *testing.T) {
	rules := []Rule{}
	ruleset := []struct {
		args         []string
		expectedType reflect.Type
	}{
		{[]string{"stop", "ttl", "srv1.coredns.rocks", "1"}, reflect.TypeFor[*exactTTLRule]()},
		{[]string{"stop", "ttl", "exact", "srv15.coredns.rocks", "15"}, reflect.TypeFor[*exactTTLRule]()},
		{[]string{"stop", "ttl", "prefix", "srv30", "30"}, reflect.TypeFor[*prefixTTLRule]()},
		{[]string{"stop", "ttl", "suffix", "45.coredns.rocks", "45"}, reflect.TypeFor[*suffixTTLRule]()},
		{[]string{"stop", "ttl", "substring", "rv50", "50"}, reflect.TypeFor[*substringTTLRule]()},
		{[]string{"stop", "ttl", "regex", `(srv10)\.(coredns)\.(rocks)`, "10"}, reflect.TypeFor[*regexTTLRule]()},
		{[]string{"stop", "ttl", "regex", `(srv20)\.(coredns)\.(rocks)`, "20"}, reflect.TypeFor[*regexTTLRule]()},
		{[]string{"stop", "ttl", "range.example.com.", "30-300"}, reflect.TypeFor[*exactTTLRule]()},
		{[]string{"stop", "ttl", "ceil.example.com.", "-11"}, reflect.TypeFor[*exactTTLRule]()},
		{[]string{"stop", "ttl", "floor.example.com.", "5-"}, reflect.TypeFor[*exactTTLRule]()},
	}
	for i, r := range ruleset {
		rule, err := newRule(r.args...)
		if err != nil {
			t.Fatalf("Rule %d: FAIL, %s: %s", i, r.args, err)
		}
		if reflect.TypeOf(rule) != r.expectedType {
			t.Fatalf("Rule %d: FAIL, %s: rule type mismatch, expected %q, but got %q", i, r.args, r.expectedType, rule)
		}
		rules = append(rules, rule)
	}
	doTTLTests(t, rules)
}

func doTTLTests(t *testing.T, rules []Rule) {
	t.Helper()
	tests := []struct {
		from      string
		fromType  uint16
		answer    []dns.RR
		ttl       uint32
		noRewrite bool
	}{
		{"srv1.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv1.coredns.rocks.  5   IN  A  10.0.0.1")}, 1, false},
		{"srv15.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv15.coredns.rocks.  5   IN  A  10.0.0.15")}, 15, false},
		{"srv30.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv30.coredns.rocks.  5   IN  A  10.0.0.30")}, 30, false},
		{"srv45.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv45.coredns.rocks.  5   IN  A  10.0.0.45")}, 45, false},
		{"srv50.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv50.coredns.rocks.  5   IN  A  10.0.0.50")}, 50, false},
		{"srv10.coredns.rocks.", dns.TypeA, []dns.RR{test.A("srv10.coredns.rocks.  5   IN  A  10.0.0.10")}, 10, false},
		{"xmpp.coredns.rocks.", dns.TypeSRV, []dns.RR{test.SRV("xmpp.coredns.rocks.  5  IN  SRV 0 100 100 srvxmpp.coredns.rocks.")}, 5, true},
		{"srv15.coredns.rocks.", dns.TypeHINFO, []dns.RR{test.HINFO("srv15.coredns.rocks.  5  HINFO INTEL-64 \"RHEL 7.5\"")}, 15, false},
		{"srv20.coredns.rocks.", dns.TypeA, []dns.RR{
			test.A("srv20.coredns.rocks.  5   IN  A  10.0.0.22"),
			test.A("srv20.coredns.rocks.  5   IN  A  10.0.0.23"),
		}, 20, false},
		{"range.example.com.", dns.TypeA, []dns.RR{test.A("range.example.com.  5   IN  A  10.0.0.1")}, 30, false},
		{"range.example.com.", dns.TypeA, []dns.RR{test.A("range.example.com.  55   IN  A  10.0.0.1")}, 55, false},
		{"range.example.com.", dns.TypeA, []dns.RR{test.A("range.example.com.  500   IN  A  10.0.0.1")}, 300, false},
		{"ceil.example.com.", dns.TypeA, []dns.RR{test.A("ceil.example.com.  5   IN  A  10.0.0.1")}, 5, false},
		{"ceil.example.com.", dns.TypeA, []dns.RR{test.A("ceil.example.com.  15   IN  A  10.0.0.1")}, 11, false},
		{"floor.example.com.", dns.TypeA, []dns.RR{test.A("floor.example.com.  0   IN  A  10.0.0.1")}, 5, false},
		{"floor.example.com.", dns.TypeA, []dns.RR{test.A("floor.example.com.  30   IN  A  10.0.0.1")}, 30, false},
	}
	ctx := context.TODO()
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.from, tc.fromType)
		m.Question[0].Qclass = dns.ClassINET
		m.Answer = tc.answer
		rw := Rewrite{
			Next:  plugin.HandlerFunc(msgPrinter),
			Rules: rules,
		}
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)
		resp := rec.Msg
		if len(resp.Answer) == 0 {
			t.Errorf("Test %d: FAIL %s (%d) Expected valid response but received %q", i, tc.from, tc.fromType, resp)
			continue
		}
		for _, a := range resp.Answer {
			if a.Header().Ttl != tc.ttl {
				t.Errorf("Test %d: FAIL %s (%d) Expected TTL to be %d but was %d", i, tc.from, tc.fromType, tc.ttl, a.Header().Ttl)
				break
			}
		}
	}
}
