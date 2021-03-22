package rewrite

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestNewFlagRule(t *testing.T) {
	tests := []struct {
		next         string
		args         []string
		expectedFail bool
	}{
		{"stop", []string{"clear", "aa"}, false},
		{"stop", []string{"clear", "ra"}, false},
		{"stop", []string{"clear", "rd"}, false},
		{"stop", []string{"set", "aa"}, false},
		{"stop", []string{"set", "ra"}, false},
		{"stop", []string{"set", "rd"}, false},
		{"continue", []string{"clear", "aa"}, false},
		{"continue", []string{"set", "rd"}, false},
		{"stop", []string{"set", "xx"}, true},
		{"stop", []string{"remove", "aa"}, true},
	}
	for i, tc := range tests {
		failed := false
		rule, err := newFlagRule(tc.next, tc.args...)
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
		tc.args = append([]string{tc.next, "flag"}, tc.args...)
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

func TestFlagRewrite(t *testing.T) {

	ctx, close := context.WithCancel(context.TODO())
	defer close()

	tests := []struct {
		next          string
		args          []string
		expectedFlag  string
		expectedValue bool
	}{
		{"stop", []string{"clear", "aa"}, authoritative, false},
		{"stop", []string{"clear", "rd"}, recursionDesired, false},
		{"stop", []string{"clear", "ra"}, recursionAvailable, false},
		{"stop", []string{"set", "aa"}, authoritative, true},
		{"stop", []string{"set", "ra"}, recursionAvailable, true},
		{"stop", []string{"set", "rd"}, recursionDesired, true},
	}

	for _, tc := range tests {
		r, err := newFlagRule(tc.next, tc.args...)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err)
		}

		rw := Rewrite{
			Next:     plugin.HandlerFunc(msgPrinter),
			Rules:    []Rule{r},
			noRevert: false,
		}

		m := new(dns.Msg)
		m.SetQuestion("coredns.rocks", dns.TypeA)
		m.Question[0].Qclass = dns.ClassINET
		m.Answer = []dns.RR{test.A("coredns.rocks.  5   IN  A  10.0.0.1")}

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err = rw.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err)
		}

		var actual bool
		switch tc.expectedFlag {
		case authoritative:
			actual = rec.Msg.Authoritative
		case recursionAvailable:
			actual = rec.Msg.RecursionAvailable
		case recursionDesired:
			actual = rec.Msg.RecursionDesired
		}

		if actual != tc.expectedValue {
			t.Fatalf("Expected rewrite flag=%s to %v, got %v", tc.expectedFlag, tc.expectedValue, actual)
		}
	}
}
