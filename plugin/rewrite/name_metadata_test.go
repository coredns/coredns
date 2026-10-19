package rewrite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestRegexNameMetadata(t *testing.T) {
	rules, err := rewriteParse(caddy.NewTestController("dns", `rewrite name regex ^([^.]+)\.example\.$ {1}.{kubernetes/client-namespace}.svc.cluster.local. metadata answer auto`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.ContextWithMetadata(t.Context())
	metadata.SetValueFunc(ctx, "kubernetes/client-namespace", func() string { return "team-a" })
	r := new(dns.Msg).SetQuestion("web.example.", dns.TypeA)
	responses, result := rules[0].Rewrite(ctx, request.Request{Req: r, W: &test.ResponseWriter{}})
	if result != RewriteDone || r.Question[0].Name != "web.team-a.svc.cluster.local." {
		t.Fatalf("result = %v, question = %v", result, r.Question[0])
	}
	answer := test.A("web.team-a.svc.cluster.local. 30 IN A 192.0.2.1")
	for _, rule := range responses {
		rule.RewriteResponse(r, answer)
	}
	if answer.Hdr.Name != "web.example." {
		t.Fatalf("answer name = %s", answer.Hdr.Name)
	}
}

func TestRegexNameMetadataValues(t *testing.T) {
	for _, tc := range []struct {
		name, replacement, value, want string
		missing                        bool
		result                         Result
	}{
		{name: "case-sensitive label", replacement: "{1}.{test/Team}.EXAMPLE.", value: "Team-A", want: "web.team-a.example.", result: RewriteDone},
		{name: "full match", replacement: "{0}{test/Team}.", value: "suffix", want: "web.example.suffix.", result: RewriteDone},
		{name: "literal metadata value", replacement: "{1}.{test/Team}.", value: "{1}.{test/Other}", want: "web.{1}.{test/other}.", result: RewriteDone},
		{name: "missing", replacement: "{1}.{test/Team}.", missing: true, want: "web.example.", result: RewriteIgnored},
		{name: "empty", replacement: "{1}.{test/Team}.", want: "web.example.", result: RewriteIgnored},
		{name: "second missing", replacement: "{1}.{test/Team}.{test/Missing}.", value: "present", want: "web.example.", result: RewriteIgnored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := newNameRule(Stop, "regex", `^([^.]+)\.example\.$`, tc.replacement, "metadata")
			if err != nil {
				t.Fatal(err)
			}
			ctx := metadata.ContextWithMetadata(t.Context())
			if !tc.missing {
				metadata.SetValueFunc(ctx, "test/Team", func() string { return tc.value })
			}
			metadata.SetValueFunc(ctx, "test/Other", func() string { return "must-not-expand" })
			r := new(dns.Msg).SetQuestion("web.example.", dns.TypeA)
			_, result := rule.Rewrite(ctx, request.Request{Req: r, W: &test.ResponseWriter{}})
			if result != tc.result || r.Question[0].Name != tc.want {
				t.Fatalf("result = %v, name = %q; want %v, %q", result, r.Question[0].Name, tc.result, tc.want)
			}
		})
	}
}

func TestRegexNameMetadataConcurrentResponses(t *testing.T) {
	rule, err := newNameRule(Stop, "regex", `^([^.]+)\.example\.$`, "{1}.{test/namespace}.svc.cluster.local.", "metadata", "answer", "auto")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 32 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			namespace := fmt.Sprintf("team-%d", i)
			ctx := metadata.ContextWithMetadata(t.Context())
			metadata.SetValueFunc(ctx, "test/namespace", func() string { return namespace })
			want := "web." + namespace + ".svc.cluster.local."
			rw := Rewrite{Rules: []Rule{rule}, Next: plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				if r.Question[0].Name != want {
					t.Errorf("upstream question = %s, want %s", r.Question[0].Name, want)
				}
				m := new(dns.Msg).SetReply(r)
				m.Answer = []dns.RR{test.CNAME(want + " 30 IN CNAME child." + want), test.A("child." + want + " 30 IN A 192.0.2.1")}
				return dns.RcodeSuccess, w.WriteMsg(m)
			})}
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			_, err := rw.ServeDNS(ctx, w, new(dns.Msg).SetQuestion("web.example.", dns.TypeA))
			if err != nil {
				t.Fatal(err)
			}
			if w.Msg == nil || len(w.Msg.Answer) != 2 {
				t.Fatalf("response = %v", w.Msg)
			}
			if w.Msg.Question[0].Name != "web.example." || w.Msg.Answer[0].Header().Name != "web.example." || w.Msg.Answer[0].(*dns.CNAME).Target != "child.web.example." || w.Msg.Answer[1].Header().Name != "child.web.example." {
				t.Fatalf("response did not revert this request's concrete names: %s", w.Msg)
			}
		})
	}
}

func TestRegexNameMetadataValidation(t *testing.T) {
	for _, replacement := range []string{"{missing}", "{test/}", "{/value}", "{test/value", "test/value}", "{{test/value}}", "{2}.{test/value}", "{-1}.{test/value}"} {
		t.Run(replacement, func(t *testing.T) {
			if _, err := newNameRule(Stop, "regex", "(.*)", replacement, "metadata"); err == nil {
				t.Fatal("invalid replacement accepted")
			}
		})
	}
	for _, match := range []string{"exact", "prefix", "suffix", "substring"} {
		if _, err := newNameRule(Stop, match, "example.", "{test/value}.", "metadata"); err == nil {
			t.Fatalf("metadata accepted for %s", match)
		}
	}
}

func TestRegexNameMetadataSkippedAndInvalid(t *testing.T) {
	for _, tc := range []struct {
		name, qname, value, want string
		wantErr                  bool
	}{
		{"missing skips stop rule", "web.example.", "", "fallback.example.", false},
		{"nonmatch", "other.test.", "unused", "other.test.", false},
		{"invalid name", "web.example.", "invalid..name", "", true},
		{"long label", "web.example.", strings.Repeat("a", 64), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := newNameRule(Stop, "regex", `^([^.]+)\.example\.$`, "{1}.{test/value}.", "metadata", "answer", "auto")
			if err != nil {
				t.Fatal(err)
			}
			fallback, err := newNameRule(Stop, "exact", "web.example.", "fallback.example.")
			if err != nil {
				t.Fatal(err)
			}
			ctx := metadata.ContextWithMetadata(t.Context())
			metadata.SetValueFunc(ctx, "test/value", func() string {
				if tc.name == "nonmatch" {
					t.Error("metadata evaluated for nonmatching name")
				}
				return tc.value
			})
			calls := 0
			rw := Rewrite{Rules: []Rule{rule, fallback}, Next: plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				calls++
				if r.Question[0].Name != tc.want {
					t.Errorf("upstream question = %s, want %s", r.Question[0].Name, tc.want)
				}
				return dns.RcodeSuccess, w.WriteMsg(new(dns.Msg).SetReply(r))
			})}
			r := new(dns.Msg).SetQuestion(tc.qname, dns.TypeA)
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			rcode, err := rw.ServeDNS(ctx, w, r)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && (rcode != dns.RcodeServerFailure || calls != 0 || r.Question[0].Name != tc.qname) {
				t.Fatal("invalid rewrite was forwarded or original question was not restored")
			}
			if !tc.wantErr && calls != 1 {
				t.Fatalf("forwarded %d times", calls)
			}
		})
	}
}

func TestRegexNameMetadataLegacyLiteral(t *testing.T) {
	rule, err := newNameRule(Stop, "regex", `^(.*)\.$`, "{1}.{test/Value}.")
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.ContextWithMetadata(t.Context())
	metadata.SetValueFunc(ctx, "test/value", func() string { t.Error("legacy rule read metadata"); return "changed" })
	r := new(dns.Msg).SetQuestion("example.", dns.TypeA)
	rule.Rewrite(ctx, request.Request{Req: r})
	if r.Question[0].Name != "example.{test/value}." {
		t.Fatalf("legacy replacement changed: %s", r.Question[0].Name)
	}
}
