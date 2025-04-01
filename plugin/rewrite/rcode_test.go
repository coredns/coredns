package rewrite

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestNewRCodeRule(t *testing.T) {
	tests := []struct {
		next         string
		args         []string
		expectedFail bool
	}{
		{"stop", []string{"numeric.rcode.coredns.rocks", "2", "0"}, false},
		{"stop", []string{"too.few.rcode.coredns.rocks", "2"}, true},
		{"stop", []string{"exact", "too.many.rcode.coredns.rocks", "2", "1", "0"}, true},
		{"stop", []string{"exact", "match.string.rcode.coredns.rocks", "SERVFAIL", "NOERROR"}, false},
		{"continue", []string{"regex", `(regex)\.rcode\.(coredns)\.(rocks)`, "FORMERR", "NOERROR"}, false},
		{"stop", []string{"invalid.rcode.coredns.rocks", "random", "nothing"}, true},
	}
	for i, tc := range tests {
		failed := false
		rule, err := newRCodeRule(tc.next, tc.args...)
		if err != nil {
			failed = true
		}
		if !failed && !tc.expectedFail {
			continue
		}
		if failed && tc.expectedFail {
			continue
		}
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v, err=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule, err)
	}
	for i, tc := range tests {
		failed := false
		tc.args = append([]string{tc.next, "rcode"}, tc.args...)
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
		t.Fatalf("Test %d: FAIL, expected fail=%t, but received fail=%t: (%s) %s, rule=%v, err=%v", i, tc.expectedFail, failed, tc.next, tc.args, rule, err)
	}
}

func TestRCodeRewrite(t *testing.T) {
	rule, err := newRCodeRule("stop", []string{"exact", "srv1.coredns.rocks", "SERVFAIL", "FORMERR"}...)

	m := new(dns.Msg)
	m.SetQuestion("srv1.coredns.rocks.", dns.TypeA)
	m.Question[0].Qclass = dns.ClassINET
	m.Answer = []dns.RR{test.A("srv1.coredns.rocks.  5   IN  A  10.0.0.1")}
	m.Rcode = dns.RcodeServerFailure
	request := request.Request{Req: m}

	rcRule, _ := rule.(*exactRCodeRule)
	var rr dns.RR
	rcRule.response.RewriteResponse(request.Req, rr)
	if request.Req.Rcode != dns.RcodeFormatError {
		t.Fatalf("RCode rewrite did not apply changes, request=%#v, err=%v", request.Req, err)
	}
}
