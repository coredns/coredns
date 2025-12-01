package rewrite

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
	ctx, close := context.WithCancel(context.TODO())
	defer close()

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
