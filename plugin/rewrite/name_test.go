package rewrite

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestRewriteIllegalName(t *testing.T) {
	r, _ := newNameRule("stop", "example.org.", "example..org.")

	rw := Rewrite{
		Next:         plugin.HandlerFunc(msgPrinter),
		Rules:        []Rule{r},
		RevertPolicy: NoRevertPolicy(),
	}

	ctx := context.TODO()
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err := rw.ServeDNS(ctx, rec, m)
	if !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("Expected invalid name, got %s", err.Error())
	}
}

func TestRewriteNamePrefixSuffix(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		expected string
	}{
		{"stop", []string{"prefix", "foo", "bar"}, "foo.example.com.", "bar.example.com."},
		{"stop", []string{"prefix", "foo.", "bar."}, "foo.example.com.", "bar.example.com."},
		{"stop", []string{"suffix", "com", "org"}, "foo.example.com.", "foo.example.org."},
		{"stop", []string{"suffix", ".com", ".org"}, "foo.example.com.", "foo.example.org."},
	}
	for _, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err)
		}

		rw := Rewrite{
			Next:         plugin.HandlerFunc(msgPrinter),
			Rules:        []Rule{r},
			RevertPolicy: NoRevertPolicy(),
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err)
		}
		actual := rec.Msg.Question[0].Name
		if actual != tc.expected {
			t.Fatalf("Expected rewrite to %v, got %v", tc.expected, actual)
		}
	}
}

func TestRewriteNameNoRewrite(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		expected string
	}{
		{"stop", []string{"prefix", "foo", "bar"}, "coredns.foo.", "coredns.foo."},
		{"stop", []string{"prefix", "foo", "bar."}, "coredns.foo.", "coredns.foo."},
		{"stop", []string{"suffix", "com", "org"}, "com.coredns.", "com.coredns."},
		{"stop", []string{"suffix", "com", "org."}, "com.coredns.", "com.coredns."},
		{"stop", []string{"substring", "service", "svc"}, "com.coredns.", "com.coredns."},
	}
	for i, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}

		rw := Rewrite{
			Next:  plugin.HandlerFunc(msgPrinter),
			Rules: []Rule{r},
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}
		actual := rec.Msg.Answer[0].Header().Name
		if actual != tc.expected {
			t.Fatalf("Test %d: Expected answer rewrite to %v, got %v", i, tc.expected, actual)
		}
	}
}

func TestRewriteNamePrefixSuffixNoAutoAnswer(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		expected string
	}{
		{"stop", []string{"prefix", "foo", "bar"}, "foo.example.com.", "bar.example.com."},
		{"stop", []string{"prefix", "foo.", "bar."}, "foo.example.com.", "bar.example.com."},
		{"stop", []string{"suffix", "com", "org"}, "foo.example.com.", "foo.example.org."},
		{"stop", []string{"suffix", ".com", ".org"}, "foo.example.com.", "foo.example.org."},
		{"stop", []string{"suffix", ".ingress.coredns.rocks", "nginx.coredns.rocks"}, "coredns.ingress.coredns.rocks.", "corednsnginx.coredns.rocks."},
	}
	for i, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}

		rw := Rewrite{
			Next:  plugin.HandlerFunc(msgPrinter),
			Rules: []Rule{r},
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}
		actual := rec.Msg.Answer[0].Header().Name
		if actual != tc.expected {
			t.Fatalf("Test %d: Expected answer rewrite to %v, got %v", i, tc.expected, actual)
		}
	}
}

func TestRewriteNamePrefixSuffixAutoAnswer(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		rewrite  string
		expected string
	}{
		{"stop", []string{"prefix", "foo", "bar", "answer", "auto"}, "foo.example.com.", "bar.example.com.", "foo.example.com."},
		{"stop", []string{"prefix", "foo.", "bar.", "answer", "auto"}, "foo.example.com.", "bar.example.com.", "foo.example.com."},
		{"stop", []string{"suffix", "com", "org", "answer", "auto"}, "foo.example.com.", "foo.example.org.", "foo.example.com."},
		{"stop", []string{"suffix", ".com", ".org", "answer", "auto"}, "foo.example.com.", "foo.example.org.", "foo.example.com."},
		{"stop", []string{"suffix", ".ingress.coredns.rocks", "nginx.coredns.rocks", "answer", "auto"}, "coredns.ingress.coredns.rocks.", "corednsnginx.coredns.rocks.", "coredns.ingress.coredns.rocks."},
	}
	for i, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}

		rw := Rewrite{
			Next:         plugin.HandlerFunc(msgPrinter),
			Rules:        []Rule{r},
			RevertPolicy: NoRestorePolicy(),
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}
		rewrite := rec.Msg.Question[0].Name
		if rewrite != tc.rewrite {
			t.Fatalf("Test %d: Expected question rewrite to %v, got %v", i, tc.rewrite, rewrite)
		}
		actual := rec.Msg.Answer[0].Header().Name
		if actual != tc.expected {
			t.Fatalf("Test %d: Expected answer rewrite to %v, got %v", i, tc.expected, actual)
		}
	}
}

func TestRewriteNameExactAnswer(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		rewrite  string
		expected string
	}{
		{"stop", []string{"exact", "coredns.rocks", "service.consul", "answer", "auto"}, "coredns.rocks.", "service.consul.", "coredns.rocks."},
		{"stop", []string{"exact", "coredns.rocks.", "service.consul.", "answer", "auto"}, "coredns.rocks.", "service.consul.", "coredns.rocks."},
		{"stop", []string{"exact", "coredns.rocks", "service.consul"}, "coredns.rocks.", "service.consul.", "coredns.rocks."},
		{"stop", []string{"exact", "coredns.rocks.", "service.consul."}, "coredns.rocks.", "service.consul.", "coredns.rocks."},
		{"stop", []string{"exact", "coredns.org.", "service.consul."}, "coredns.rocks.", "coredns.rocks.", "coredns.rocks."},
	}
	for i, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}

		rw := Rewrite{
			Next:         plugin.HandlerFunc(msgPrinter),
			Rules:        []Rule{r},
			RevertPolicy: NoRestorePolicy(),
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}
		rewrite := rec.Msg.Question[0].Name
		if rewrite != tc.rewrite {
			t.Fatalf("Test %d: Expected question rewrite to %v, got %v", i, tc.rewrite, rewrite)
		}
		actual := rec.Msg.Answer[0].Header().Name
		if actual != tc.expected {
			t.Fatalf("Test %d: Expected answer rewrite to %v, got %v", i, tc.expected, actual)
		}
	}
}

func TestRewriteNameRegexAnswer(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		next     string
		args     []string
		question string
		rewrite  string
		expected string
	}{
		{"stop", []string{"regex", "(.*).coredns.rocks", "{1}.coredns.maps", "answer", "auto"}, "foo.coredns.rocks.", "foo.coredns.maps.", "foo.coredns.rocks."},
		{"stop", []string{"regex", "(.*).coredns.rocks", "{1}.coredns.maps", "answer", "name", "(.*).coredns.maps", "{1}.coredns.works"}, "foo.coredns.rocks.", "foo.coredns.maps.", "foo.coredns.works."},
		{"stop", []string{"regex", "(.*).coredns.rocks", "{1}.coredns.maps"}, "foo.coredns.rocks.", "foo.coredns.maps.", "foo.coredns.maps."},
	}
	for i, tc := range tests {
		r, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}

		rw := Rewrite{
			Next:         plugin.HandlerFunc(msgPrinter),
			Rules:        []Rule{r},
			RevertPolicy: NoRestorePolicy(),
		}

		m := new(dns.Msg)
		m.SetQuestion(tc.question, dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %s", i, err)
		}
		rewrite := rec.Msg.Question[0].Name
		if rewrite != tc.rewrite {
			t.Fatalf("Test %d: Expected question rewrite to %v, got %v", i, tc.rewrite, rewrite)
		}
		actual := rec.Msg.Answer[0].Header().Name
		if actual != tc.expected {
			t.Fatalf("Test %d: Expected answer rewrite to %v, got %v", i, tc.expected, actual)
		}
	}
}

func TestNewNameRule(t *testing.T) {
	tests := []struct {
		next         string
		args         []string
		expectedFail bool
	}{
		{"stop", []string{"exact", "srv3.coredns.rocks", "srv4.coredns.rocks"}, false},
		{"stop", []string{"srv1.coredns.rocks", "srv2.coredns.rocks"}, false},
		{"stop", []string{"suffix", "coredns.rocks", "coredns.rocks."}, false},
		{"stop", []string{"suffix", "coredns.rocks.", "coredns.rocks"}, false},
		{"stop", []string{"suffix", "coredns.rocks.", "coredns.rocks."}, false},
		{"stop", []string{"regex", "srv1.coredns.rocks", "10"}, false},
		{"stop", []string{"regex", "(.*).coredns.rocks", "10"}, false},
		{"stop", []string{"regex", "(.*).coredns.rocks", "{1}.coredns.rocks"}, false},
		{"stop", []string{"regex", "(.*).coredns.rocks", "{1}.{2}.coredns.rocks"}, true},
		{"stop", []string{"regex", "staging.mydomain.com", "aws-loadbalancer-id.us-east-1.elb.amazonaws.com"}, false},
		{"stop", []string{"suffix", "staging.mydomain.com", "coredns.rock", "answer"}, true},
		{"stop", []string{"suffix", "staging.mydomain.com", "coredns.rock", "answer", "name"}, true},
		{"stop", []string{"suffix", "staging.mydomain.com", "coredns.rock", "answer", "other"}, true},
		{"stop", []string{"suffix", "staging.mydomain.com", "coredns.rock", "answer", "auto"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "auto"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name"}, true},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "coredns.rock", "staging.mydomain.com"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.{2}.staging.mydomain.com"}, true},

		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "name", "(.*).coredns.rock"}, true},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"regex", "staging.mydomain.com", "coredns.rock", "answer", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com", "value", "(.*).coredns.rock"}, true},

		{"stop", []string{"suffix", "staging.mydomain.com.", "coredns.rock.", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"suffix", "staging.mydomain.com.", "coredns.rock.", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"suffix", "staging.mydomain.com.", "coredns.rock.", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com", "name", "(.*).coredns.rock", "{1}.staging.mydomain.com"}, false},
		{"stop", []string{"suffix", "staging.mydomain.com.", "coredns.rock.", "answer", "value", "(.*).coredns.rock", "{1}.staging.mydomain.com", "value", "(.*).coredns.rock"}, true},
	}
	for i, tc := range tests {
		failed := false
		rule, err := newNameRule(tc.next, tc.args...)
		if err != nil {
			failed = true
		}
		if !failed && !tc.expectedFail {
			t.Logf("Test %d: PASS, passed as expected: (%s) %s", i, tc.next, tc.args)
			continue
		}
		if failed && tc.expectedFail {
			t.Logf("Test %d: PASS, failed as expected: (%s) %s: %s", i, tc.next, tc.args, err)
			continue
		}
		if failed && !tc.expectedFail {
			t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v, error=%s", i, tc.expectedFail, failed, tc.next, tc.args, rule, err)
		}
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule)
	}
	for i, tc := range tests {
		failed := false
		tc.args = append([]string{tc.next, "name"}, tc.args...)
		rule, err := newRule(tc.args...)
		if err != nil {
			failed = true
		}
		if !failed && !tc.expectedFail {
			t.Logf("Test %d: PASS, passed as expected: (%s) %s", i, tc.next, tc.args)
			continue
		}
		if failed && tc.expectedFail {
			t.Logf("Test %d: PASS, failed as expected: (%s) %s: %s", i, tc.next, tc.args, err)
			continue
		}
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule)
	}
}

func TestNewNameRuleLargeRegex(t *testing.T) {
	largeRegex := strings.Repeat("a", maxRegexpLen+1)
	_, err := newNameRule("stop", "regex", largeRegex, "replacement")
	if err == nil {
		t.Fatal("Expected error for large regex, got nil")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("Expected 'too long' error, got: %v", err)
	}
}

func TestRemapStringRewriter(t *testing.T) {
	r := newRemapStringRewriter("example.com.", "example.org.")

	tests := []struct {
		src      string
		expected string
	}{
		{"example.com.", "example.org."},         // the name itself
		{"sub.example.com.", "sub.example.org."}, // a sub domain
		{"a.b.example.com.", "a.b.example.org."},
		{"notexample.com.", "notexample.com."}, // same suffix, no label boundary
		{".example.com.", ".example.org."},
		{"example.net.", "example.net."},
		{"com.", "com."}, // shorter than orig
		{"", ""},
	}
	for _, tc := range tests {
		if got := r.rewriteString(tc.src); got != tc.expected {
			t.Errorf("rewriteString(%q) = %q, expected %q", tc.src, got, tc.expected)
		}
	}
}

// k8sName is a Kubernetes service name of the length these routinely reach. It
// matters because it is longer than 31 bytes: Go concatenates short strings into
// a 32-byte stack buffer, so "."+orig stayed off the heap for a name like
// example.com. but had to be allocated for one this long, once per call.
const k8sName = "my-service.my-namespace.svc.cluster.local." // 42 bytes

// BenchmarkRemapStringRewriter measures a single rewriteString call on a rewriter
// that already exists. An auto rule builds a fresh rewriter per matching request
// and then calls it once per record, so this is the per-record cost only — see
// BenchmarkAutoNameRuleResponse for what one whole request costs.
//
// orig is the name the question was rewritten to, so its length is a property of
// the Corefile, and it decides whether the old "."+orig temporary allocated.
func BenchmarkRemapStringRewriter(b *testing.B) {
	cases := []struct{ name, orig, src string }{
		{"short/match", "example.com.", "sub.example.com."},
		{"short/nomatch", "example.com.", "sub.example.net."},
		{"long/match", k8sName, "pod-1234." + k8sName},
		// Same length as orig, so this reaches the byte comparison rather than
		// being rejected on length alone.
		{"long/nomatch", k8sName, "my-service.my-namespace.svc.cluster.zzzzz."},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r := newRemapStringRewriter(tc.orig, "example.org.")
			b.ReportAllocs()
			for b.Loop() {
				remapSink = r.rewriteString(tc.src)
			}
		})
	}
}

// BenchmarkAutoNameRuleResponse measures one whole request through an auto name
// rule: rewrite the question, build the response rules for it — which is where the
// rewriter is constructed — and apply them to the answer a backend would return.
// This is the shape that decides whether an optimization in rewriteString is worth
// what it costs newRemapStringRewriter, since neither survives the request.
//
// The sub cases are the paths a record owner can take through the rewriter. Only
// the sub domain ones reach the label boundary check; an owner equal to the
// rewritten question name returns the replacement without comparing anything, so
// that case measures construction and nothing else. The k8s cases rewrite to a
// name past the 32-byte concat buffer, where the old temporary had to allocate.
func BenchmarkAutoNameRuleResponse(b *testing.B) {
	subdomains := func(to string, n int) []string {
		owners := make([]string, n)
		for i := range owners {
			owners[i] = string(rune('a'+i)) + "." + to
		}
		return owners
	}

	cases := []struct {
		name  string
		from  string
		to    string
		owner []string
	}{
		{"exact", "example.com.", "example.org.", []string{"example.org."}},
		{"subdomain", "example.com.", "example.org.", []string{"sub.example.org."}},
		{"nomatch", "example.com.", "example.org.", []string{"sub.example.net."}},
		{"subdomain-8", "example.com.", "example.org.", subdomains("example.org.", 8)},
		{"k8s/subdomain", "svc.example.com.", k8sName, []string{"pod-1234." + k8sName}},
		{"k8s/subdomain-8", "svc.example.com.", k8sName, subdomains(k8sName, 8)},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			rule, err := newNameRule("stop", "exact", tc.from, tc.to, "answer", "auto")
			if err != nil {
				b.Fatal(err)
			}
			ctx := b.Context()
			m := new(dns.Msg)
			m.SetQuestion(tc.from, dns.TypeA)
			answer := make([]dns.RR, len(tc.owner))
			for i, owner := range tc.owner {
				answer[i] = test.A(owner + " 3600 IN A 127.0.0.1")
			}

			b.ReportAllocs()
			for b.Loop() {
				// Both rules rewrite in place, so restore the request and the
				// answer the backend would have returned for it.
				m.Question[0].Name = tc.from
				for i, rr := range answer {
					rr.Header().Name = tc.owner[i]
				}

				rules, _ := rule.Rewrite(ctx, request.Request{Req: m})
				for _, rr := range answer {
					for _, rule := range rules {
						rule.RewriteResponse(m, rr)
					}
				}
				rulesSink = rules
			}
		})
	}
}

// remapSink keeps the rewritten string live; assigning to _ lets the compiler
// drop the concatenation and the benchmark then reports work it never did.
var (
	remapSink string
	rulesSink ResponseRules
)
