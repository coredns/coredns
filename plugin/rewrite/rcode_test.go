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
		{"stop", []string{"srv1.coredns.rocks", "2", "0"}, false},
		{"stop", []string{"exact", "srv1.coredns.rocks", "SERVFAIL", "NOERROR"}, false},
		{"stop", []string{"prefix", "coredns.rocks", "NotAuth", "0"}, false},
		{"stop", []string{"suffix", "srv1", "SERVFAIL", "0"}, false},
		{"stop", []string{"substring", "coredns", "REFUSED", "NoError"}, false},
		{"stop", []string{"regex", `(srv1)\.(coredns)\.(rocks)`, "FORMERR", "NOERROR"}, false},
		{"continue", []string{"srv1.coredns.rocks", "2", "0"}, false},
		{"continue", []string{"exact", "srv1.coredns.rocks", "SERVFAIL", "NOERROR"}, false},
		{"continue", []string{"prefix", "coredns.rocks", "NOTAUTH", "0"}, false},
		{"continue", []string{"suffix", "srv1", "SERVFAIL", "0"}, false},
		{"continue", []string{"substring", "coredns", "REFUSED", "NOERROR"}, false},
		{"continue", []string{"regex", `(srv1)\.(coredns)\.(rocks)`, "FORMERR", "NOERROR"}, false},
		{"stop", []string{"srv1.coredns.rocks", "12345678901234567890"}, true},
		{"stop", []string{"srv1.coredns.rocks", "coredns.rocks"}, true},
		{"stop", []string{"srv1.coredns.rocks", "#1"}, true},
		{"stop", []string{"range.coredns.rocks", "1", "2"}, false},
		{"stop", []string{"ceil.coredns.rocks", "-2"}, true},
		{"stop", []string{"floor.coredns.rocks", "1-"}, true},
		{"stop", []string{"range.coredns.rocks", "2", "2"}, false},
		{"stop", []string{"invalid.coredns.rocks", "-"}, true},
		{"stop", []string{"invalid.coredns.rocks", "2-1"}, true},
		{"stop", []string{"invalid.coredns.rocks", "random"}, true},
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
	m.MsgHdr.Rcode = dns.RcodeServerFailure
	request := request.Request{Req: m}

	rcRule, _ := rule.(*exactRCodeRule)
	var rr dns.RR
	rcRule.response.RewriteResponse(request.Req, rr)
	if request.Req.MsgHdr.Rcode != dns.RcodeFormatError {
		t.Fatalf("RCode rewrite did not apply changes, request=%#v, err=%v", request.Req, err)
	}
}
