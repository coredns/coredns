package rewrite

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestPartialRewrite(t *testing.T) {
	tests := []struct {
		name, match, from, to, query, rewritten string
	}{
		{"prefix within label", "prefix", "API-", "SVC-", "api-prod.example.", "svc-prod.example."},
		{"substring within label", "substring", "-API-", "-SVC-", "my-api-prod.example.", "my-svc-prod.example."},
		{"prefix replacement without dot", "prefix", "API.", "SVC-", "api.prod.example.", "svc-prod.example."},
		{"substring replacement without dot", "substring", "API.", "SVC-", "my.api.prod.example.", "my.svc-prod.example."},
		{"explicit prefix dots", "prefix", "API.", "SVC.", "api.prod.example.", "svc.prod.example."},
		{"explicit substring dots", "substring", "API.", "SVC.", "my.api.prod.example.", "my.svc.prod.example."},
		{"exact name", "exact", "API.PROD.EXAMPLE", "SVC.PROD.EXAMPLE", "api.prod.example.", "svc.prod.example."},
		{"suffix name", "suffix", "PROD.EXAMPLE", "STAGING.EXAMPLE", "api.prod.example.", "api.staging.example."},
	}
	for _, field := range []string{"name", "ttl", "rcode"} {
		for _, tc := range tests {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				args := []string{"stop", field, tc.match, tc.from}
				switch field {
				case "name":
					args = append(args, tc.to)
				case "ttl":
					args = append(args, "60")
				case "rcode":
					args = append(args, "NOERROR", "NXDOMAIN")
				}
				rule, err := newRule(args...)
				if err != nil {
					t.Fatal(err)
				}
				var forwardedName string
				next := plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					forwardedName = r.Question[0].Name
					res := new(dns.Msg).SetReply(r)
					res.Answer = []dns.RR{test.A(forwardedName + " 300 IN A 192.0.2.1")}
					return 0, w.WriteMsg(res)
				})
				req := new(dns.Msg).SetQuestion(tc.query, dns.TypeA)
				res := serveEdns0Rewrite(t, rule, next, req)
				if res == nil {
					t.Fatal("no response was written")
				}
				switch field {
				case "name":
					if forwardedName != tc.rewritten {
						t.Errorf("forwarded name = %q, want %q", forwardedName, tc.rewritten)
					}
				case "ttl":
					if got := res.Answer[0].Header().Ttl; got != 60 {
						t.Errorf("TTL = %d, want 60", got)
					}
				case "rcode":
					if res.Rcode != dns.RcodeNameError {
						t.Errorf("RCODE = %d, want NXDOMAIN", res.Rcode)
					}
				}
			})
		}
	}
}
