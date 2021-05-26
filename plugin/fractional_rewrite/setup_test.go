package fractional_rewrite

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	tst "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/whoami"
	"github.com/miekg/dns"
	"gotest.tools/assert"
)

func TestRewriteParse(t *testing.T) {
	tests := []struct {
		testConfig  string
		isValid     bool
		description string
	}{
		{
			`fractional_rewrite suffix 0.1 fabric.dog fabric.dog-canary`,
			true,
			"legal case",
		},
		{
			`fractional_rewrite prefix 0.1 a.com aa.com`,
			false,
			"specified rule is not implemented",
		},
		{
			`fractional_rewrite 0.2 a.com a.com.cn`,
			false,
			"missing args",
		},
	}
	for _, test := range tests {
		c := caddy.NewTestController("dns", test.testConfig)
		_, err := fractionalRewriteParse(c)
		if test.isValid != (err == nil) {
			t.Errorf("fractional_rewrite doesn't parse correctly: %s", test.description)
		}
	}
}

func TestRewriteRule(t *testing.T) {
	tests := []struct {
		fromQ    string
		toQ      string
		fraction string
	}{
		{"a.fabric.dog", "a.fabric.dog", "0.0"},
		{"a.com", "a.com", "0.0"},
		{"abc.fabric.dog", "abc.fabric.dog-canary", "1.0"},
		{"a.com", "a.com", "1.0"},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", fmt.Sprintf(`fractional_rewrite suffix %s fabric.dog fabric.dog-canary`, test.fraction))
		r, err := fractionalRewriteParse(c)
		assert.NilError(t, err)
		fr := fractionalRewrite{
			Next: whoami.Whoami{},
			Rule: r,
		}
		ctx := context.TODO()
		m := new(dns.Msg)
		m.SetQuestion(test.fromQ, dns.TypeA)

		rec := dnstest.NewRecorder(&tst.ResponseWriter{})
		fr.ServeDNS(ctx, rec, m)

		resp := rec.Msg
		if resp.Question[0].Name != test.toQ {
			t.Errorf("Test %d: Expected Name to be %q but was %q", i, test.toQ, resp.Question[0].Name)
		}
	}
}
