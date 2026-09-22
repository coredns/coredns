package template

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestExpr(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		qname          string
		qtype          uint16
		md             map[string]string
		expectedCode   int
		expectedAnswer []string
	}{
		{
			name: "True",
			config: `template IN A example. {
				expr name() == 'a.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "FalseServfail",
			config: `template IN A example. {
				expr name() == 'b.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
			}`,
			qname:        "a.example.",
			expectedCode: dns.RcodeServerFailure,
		},
		{
			name: "FalseFallthrough",
			config: `template IN A example. {
				expr name() == 'b.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "a.example.",
			expectedCode: rcodeFallthrough,
		},
		{
			name: "FalseFallthroughZoneMismatch",
			config: `template IN A example. {
				expr name() == 'b.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough other.example.
			}`,
			qname:        "a.example.",
			expectedCode: dns.RcodeServerFailure,
		},
		{
			name: "FalseFallsToNextTemplate",
			config: `template IN A example. {
				expr name() == 'b.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}
			template IN A example. {
				answer "{{ .Name }} 60 IN A 10.0.0.2"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.2"},
		},
		{
			name: "AllTrue",
			config: `template IN A example. {
				expr name() == 'a.example.'
				expr type() == 'A'
				expr incidr(client_ip(), '10.0.0.0/8')
				answer "{{ .Name }} 60 IN A 10.0.0.1"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "LastFalse",
			config: `template IN A example. {
				expr name() == 'a.example.'
				expr type() == 'A'
				expr incidr(client_ip(), '192.0.2.0/24')
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "a.example.",
			expectedCode: rcodeFallthrough,
		},
		{
			name: "UsesVars",
			config: `template IN A example. {
				match ^(?P<n>[0-9]+)[.]example[.]$
				var num int(group('n'))
				expr num > 1
				answer "{{ .Name }} 60 IN A 10.0.0.{{ .Var.num }}"
				fallthrough
			}`,
			qname:          "2.example.",
			expectedAnswer: []string{"2.example. 60 IN A 10.0.0.2"},
		},
		{
			name: "UsesVarsFalse",
			config: `template IN A example. {
				match ^(?P<n>[0-9]+)[.]example[.]$
				var num int(group('n'))
				expr num > 1
				answer "{{ .Name }} 60 IN A 10.0.0.{{ .Var.num }}"
				fallthrough
			}`,
			qname:        "1.example.",
			expectedCode: rcodeFallthrough,
		},
		{
			name: "UsesGroup",
			config: `template IN A example. {
				match ^(?P<n>[0-9]+)[.]example[.]$
				expr group('n') == '2' && group(0) == '2.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.{{ .Group.n }}"
				fallthrough
			}`,
			qname:          "2.example.",
			expectedAnswer: []string{"2.example. 60 IN A 10.0.0.2"},
		},
		{
			name: "Metadata",
			config: `template IN A example. {
				expr metadata('test/region') == 'eu'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:          "a.example.",
			md:             map[string]string{"test/region": "eu"},
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "MetadataMismatch",
			config: `template IN A example. {
				expr metadata('test/region') == 'eu'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "a.example.",
			md:           map[string]string{"test/region": "us"},
			expectedCode: rcodeFallthrough,
		},
		{
			name: "NonBool",
			config: `template IN A example. {
				expr 1
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "a.example.",
			expectedCode: rcodeFallthrough,
		},
		{
			name: "RuntimeError",
			config: `template IN A example. {
				expr incidr('notanip', '10.0.0.0/8')
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "a.example.",
			expectedCode: dns.RcodeServerFailure,
		},
		{
			name: "NoMatchNoEvaluation",
			config: `template IN A example. {
				match ^a[.]example[.]$
				expr incidr('notanip', '10.0.0.0/8')
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				fallthrough
			}`,
			qname:        "b.example.",
			expectedCode: rcodeFallthrough,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.config)
			handler, err := templateParse(c)
			if err != nil {
				t.Fatalf("expected no config error, got: %v", err)
			}
			handler.Next = test.NextHandler(rcodeFallthrough, nil)

			ctx := context.Background()
			if tc.md != nil {
				ctx = metadata.ContextWithMetadata(ctx)
				for k, v := range tc.md {
					value := v
					metadata.SetValueFunc(ctx, k, func() string { return value })
				}
			}

			qtype := tc.qtype
			if qtype == 0 {
				qtype = dns.TypeA
			}
			req := &dns.Msg{Question: []dns.Question{{Name: tc.qname, Qclass: dns.ClassINET, Qtype: qtype}}}
			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			code, err := handler.ServeDNS(ctx, rec, req)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if code != tc.expectedCode {
				t.Fatalf("expected rcode %v, got %v", tc.expectedCode, code)
			}
			if tc.expectedCode != dns.RcodeSuccess {
				return
			}

			verifySection(t, "answer", rec.Msg.Answer, tc.expectedAnswer)
		})
	}
}

func TestExprMatch(t *testing.T) {
	tests := []struct {
		name             string
		config           string
		qname            string
		expectedMatch    bool
		expectedFthrough bool
	}{
		{
			name: "AllTrue",
			config: `template IN A example. {
				expr name() == 'a.example.'
				expr true
				fallthrough
			}`,
			qname:         "a.example.",
			expectedMatch: true,
		},
		{
			name: "FirstFalse",
			config: `template IN A example. {
				expr false
				expr true
				fallthrough
			}`,
			qname:            "a.example.",
			expectedFthrough: true,
		},
		{
			name: "FalseWithoutFallthrough",
			config: `template IN A example. {
				expr false
			}`,
			qname: "a.example.",
		},
		{
			name: "ErrorWithFallthrough",
			config: `template IN A example. {
				expr incidr('notanip', '10.0.0.0/8')
				fallthrough
			}`,
			qname: "a.example.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.config)
			handler, err := templateParse(c)
			if err != nil {
				t.Fatalf("expected no config error, got: %v", err)
			}

			req := &dns.Msg{Question: []dns.Question{{Name: tc.qname, Qclass: dns.ClassINET, Qtype: dns.TypeA}}}
			_, match, fthrough := handler.Templates[0].match(context.Background(), requestFor(req))
			if match != tc.expectedMatch {
				t.Fatalf("expected match %v, got %v", tc.expectedMatch, match)
			}
			if fthrough != tc.expectedFthrough {
				t.Fatalf("expected fallthrough %v, got %v", tc.expectedFthrough, fthrough)
			}
		})
	}
}
