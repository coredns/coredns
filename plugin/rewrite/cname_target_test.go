package rewrite

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type MockedUpstream struct{}

func (u *MockedUpstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetReply(state.Req)
	m.Authoritative = true
	switch state.Req.Question[0].Name {
	case "xyz.example.com.":
		switch state.Req.Question[0].Qtype {
		case dns.TypeA:
			m.Answer = []dns.RR{
				test.A("xyz.example.com.  3600  IN  A 3.4.5.6"),
			}
		case dns.TypeAAAA:
			m.Answer = []dns.RR{
				test.AAAA("xyz.example.com.  3600  IN  AAAA 3a01:7e00::f03c:91ff:fe79:234c"),
			}
		}
		return m, nil
	case "bard.google.com.cdn.cloudflare.net.":
		m.Answer = []dns.RR{
			test.A("bard.google.com.cdn.cloudflare.net.  1800  IN  A  9.7.2.1"),
		}
		return m, nil
	case "www.hosting.xyz.":
		m.Answer = []dns.RR{
			test.A("www.hosting.xyz.  500  IN  A  20.30.40.50"),
		}
		return m, nil
	case "abcd.zzzz.www.pqrst.":
		m.Answer = []dns.RR{
			test.A("abcd.zzzz.www.pqrst.   120  IN  A   101.20.5.1"),
			test.A("abcd.zzzz.www.pqrst.   120  IN  A   101.20.5.2"),
		}
		return m, nil
	case "orders.webapp.eu.org.":
		m.Answer = []dns.RR{
			test.A("orders.webapp.eu.org.   120  IN  A   20.0.0.9"),
		}
		return m, nil
	case "music.truncated.spotify.com.":
		m.Answer = []dns.RR{
			test.A("music.truncated.spotify.com.   120  IN  A   10.1.0.9"),
		}
		m.Truncated = true
		return m, nil
	}
	return &dns.Msg{}, nil
}

func TestCNameTargetRewrite(t *testing.T) {
	rules := []Rule{}
	ruleset := []struct {
		args         []string
		expectedType reflect.Type
	}{
		{[]string{"continue", "cname", "exact", "def.example.com.", "xyz.example.com."}, reflect.TypeFor[*cnameTargetRule]()},
		{[]string{"continue", "cname", "prefix", "chat.openai.com", "bard.google.com"}, reflect.TypeFor[*cnameTargetRule]()},
		{[]string{"continue", "cname", "suffix", "uvw.", "xyz."}, reflect.TypeFor[*cnameTargetRule]()},
		{[]string{"continue", "cname", "substring", "efgh", "zzzz.www"}, reflect.TypeFor[*cnameTargetRule]()},
		{[]string{"continue", "cname", "regex", `(.*)\.web\.(.*)\.site\.`, `{1}.webapp.{2}.org.`}, reflect.TypeFor[*cnameTargetRule]()},
		{[]string{"continue", "cname", "exact", "music.truncated.spotify.com.", "music.truncated.spotify.com."}, reflect.TypeFor[*cnameTargetRule]()},
	}
	for i, r := range ruleset {
		rule, err := newRule(r.args...)
		if err != nil {
			t.Fatalf("Rule %d: FAIL, %s: %s", i, r.args, err)
		}
		if reflect.TypeOf(rule) != r.expectedType {
			t.Fatalf("Rule %d: FAIL, %s: rule type mismatch, expected %q, but got %q", i, r.args, r.expectedType, rule)
		}
		cnameTargetRule := rule.(*cnameTargetRule)
		cnameTargetRule.Upstream = &MockedUpstream{}
		rules = append(rules, rule)
	}
	doTestCNameTargetTests(t, rules)
}

func doTestCNameTargetTests(t *testing.T, rules []Rule) {
	t.Helper()
	tests := []struct {
		from              string
		fromType          uint16
		answer            []dns.RR
		expectedAnswer    []dns.RR
		expectedTruncated bool
	}{
		{"abc.example.com", dns.TypeA,
			[]dns.RR{
				test.CNAME("abc.example.com.  5   IN  CNAME  def.example.com."),
				test.A("def.example.com.   5  IN  A   1.2.3.4"),
			},
			[]dns.RR{
				test.CNAME("abc.example.com.  5   IN  CNAME  xyz.example.com."),
				test.A("xyz.example.com.  3600  IN  A  3.4.5.6"),
			},
			false,
		},
		{"abc.example.com", dns.TypeAAAA,
			[]dns.RR{
				test.CNAME("abc.example.com.  5   IN  CNAME  def.example.com."),
				test.AAAA("def.example.com.   5  IN  AAAA   2a01:7e00::f03c:91ff:fe79:234c"),
			},
			[]dns.RR{
				test.CNAME("abc.example.com.  5   IN  CNAME  xyz.example.com."),
				test.AAAA("xyz.example.com.  3600  IN  AAAA  3a01:7e00::f03c:91ff:fe79:234c"),
			},
			false,
		},
		{"chat.openai.com", dns.TypeA,
			[]dns.RR{
				test.CNAME("chat.openai.com.  20   IN  CNAME  chat.openai.com.cdn.cloudflare.net."),
				test.A("chat.openai.com.cdn.cloudflare.net.   30  IN  A   23.2.1.2"),
				test.A("chat.openai.com.cdn.cloudflare.net.   30  IN  A   24.6.0.8"),
			},
			[]dns.RR{
				test.CNAME("chat.openai.com.  20   IN  CNAME  bard.google.com.cdn.cloudflare.net."),
				test.A("bard.google.com.cdn.cloudflare.net.  1800  IN  A  9.7.2.1"),
			},
			false,
		},
		{"coredns.io", dns.TypeA,
			[]dns.RR{
				test.CNAME("coredns.io.  100   IN  CNAME  www.hosting.uvw."),
				test.A("www.hosting.uvw.   200  IN  A   7.2.3.4"),
			},
			[]dns.RR{
				test.CNAME("coredns.io.  100   IN  CNAME  www.hosting.xyz."),
				test.A("www.hosting.xyz.  500  IN  A  20.30.40.50"),
			},
			false,
		},
		{"core.dns.rocks", dns.TypeA,
			[]dns.RR{
				test.CNAME("core.dns.rocks.  200   IN  CNAME  abcd.efgh.pqrst."),
				test.A("abcd.efgh.pqrst.   100  IN  A   200.30.45.67"),
			},
			[]dns.RR{
				test.CNAME("core.dns.rocks.  200   IN  CNAME  abcd.zzzz.www.pqrst."),
				test.A("abcd.zzzz.www.pqrst.   120  IN  A   101.20.5.1"),
				test.A("abcd.zzzz.www.pqrst.   120  IN  A   101.20.5.2"),
			},
			false,
		},
		{"order.service.eu", dns.TypeA,
			[]dns.RR{
				test.CNAME("order.service.eu.  200   IN  CNAME  orders.web.eu.site."),
				test.A("orders.web.eu.site.   50  IN  A   10.10.15.1"),
			},
			[]dns.RR{
				test.CNAME("order.service.eu.  200   IN  CNAME  orders.webapp.eu.org."),
				test.A("orders.webapp.eu.org.   120  IN  A   20.0.0.9"),
			},
			false,
		},
		{"music.spotify.com", dns.TypeA,
			[]dns.RR{
				test.CNAME("music.spotify.com.  200   IN  CNAME  music.truncated.spotify.com."),
			},
			[]dns.RR{
				test.CNAME("music.spotify.com.  200   IN  CNAME  music.truncated.spotify.com."),
				test.A("music.truncated.spotify.com.   120  IN  A   10.1.0.9"),
			},
			true,
		},
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
		if !reflect.DeepEqual(resp.Answer, tc.expectedAnswer) {
			t.Errorf("Test %d: FAIL %s (%d) Actual are expected answer does not match, actual: %v, expected: %v",
				i, tc.from, tc.fromType, resp.Answer, tc.expectedAnswer)
			continue
		}
		if resp.Truncated != tc.expectedTruncated {
			t.Errorf("Test %d: FAIL %s (%d) Actual and expected truncated flag do not match, actual: %v, expected: %v",
				i, tc.from, tc.fromType, resp.Truncated, tc.expectedTruncated)
		}
	}
}

// nilUpstream returns a nil message to simulate an upstream failure path.
type nilUpstream struct{}

func (f *nilUpstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return nil, nil
}

// errUpstream returns a nil message with an error to simulate an upstream failure path.
type errUpstream struct{}

func (f *errUpstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return nil, errors.New("upstream failure")
}

func TestCNAMETargetRewrite_upstreamFailurePaths(t *testing.T) {
	cases := []struct {
		name     string
		upstream UpstreamInt
	}{
		{name: "nil message, no error", upstream: &nilUpstream{}},
		{name: "nil message, with error", upstream: &errUpstream{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := cnameTargetRule{
				rewriteType:     ExactMatch,
				paramFromTarget: "bad.target.",
				paramToTarget:   "good.target.",
				nextAction:      Stop,
				Upstream:        tc.upstream,
			}

			req := new(dns.Msg)
			req.SetQuestion("bad.test.", dns.TypeA)
			state := request.Request{Req: req}

			rrState := &cnameTargetRuleWithReqState{rule: rule, state: state, ctx: context.Background()}

			res := new(dns.Msg)
			res.SetReply(req)
			res.Answer = []dns.RR{&dns.CNAME{Hdr: dns.RR_Header{Name: "bad.test.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60}, Target: "bad.target."}}

			rr := &dns.CNAME{Hdr: dns.RR_Header{Name: "bad.test.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60}, Target: "bad.target."}

			rrState.RewriteResponse(res, rr)

			finalTarget := res.Answer[0].(*dns.CNAME).Target
			if finalTarget != "bad.target." {
				t.Errorf("Expected answer to be %q, but got %q", "bad.target.", finalTarget)
			}
		})
	}
}
