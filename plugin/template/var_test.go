package template

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestVar(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		qname          string
		qtype          uint16
		md             map[string]string
		expectedCode   int
		expectedErr    bool
		expectedAnswer []string
		expectedExtra  []string
		expectedNs     []string
	}{
		{
			name: "Constant",
			config: `template IN A example. {
				var ttl 60
				answer "{{ .Name }} {{ .Var.ttl }} IN A 10.0.0.1"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "Chained",
			config: `template IN A example. {
				var a 1
				var b a + 1
				var c b * 10
				answer "{{ .Name }} 60 IN A 10.{{ .Var.a }}.{{ .Var.b }}.{{ .Var.c }}"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.1.2.20"},
		},
		{
			name: "GroupByNameAndIndex",
			config: `template IN A example. {
				match ^(?P<a>[0-9]+)-(?P<b>[0-9]+)[.]example[.]$
				var a group('a')
				var b group(2)
				var whole group(0)
				answer "{{ .Var.whole }} 60 IN A 10.0.{{ .Var.a }}.{{ .Var.b }}"
			}`,
			qname:          "12-34.example.",
			expectedAnswer: []string{"12-34.example. 60 IN A 10.0.12.34"},
		},
		{
			name: "GroupMissing",
			config: `template IN TXT example. {
				match ^(?P<a>[a-z]+)[.]example[.]$
				var missing group('nosuchgroup') + group(9)
				answer "{{ .Name }} 60 IN TXT \"empty={{ eq .Var.missing \"\" }}\""
			}`,
			qname:          "a.example.",
			qtype:          dns.TypeTXT,
			expectedAnswer: []string{`a.example. 60 IN TXT "empty=true"`},
		},
		{
			name: "QueryFunctions",
			config: `template IN A example. {
				var n name()
				var t type()
				var ip client_ip()
				answer "{{ .Var.n }} 60 IN A {{ .Var.ip }}"
				additional "{{ .Var.n }} 60 IN TXT \"{{ .Var.t }}\""
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.240.0.1"},
			expectedExtra:  []string{`a.example. 60 IN TXT "A"`},
		},
		{
			name: "Metadata",
			config: `template IN A example. {
				var region metadata('test/region')
				answer "{{ .Var.region }}.{{ .Name }} 60 IN A 10.0.0.1"
			}`,
			qname:          "a.example.",
			md:             map[string]string{"test/region": "eu"},
			expectedAnswer: []string{"eu.a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "BoolInTemplateCondition",
			config: `template IN A example. {
				var local incidr(client_ip(), '10.0.0.0/8')
				answer "{{ .Name }} 60 IN A {{ if .Var.local }}10.0.0.1{{ else }}192.0.2.1{{ end }}"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "UsedInAllSections",
			config: `template IN A example. {
				var target 'ns0.example.'
				answer "{{ .Name }} 60 IN A 10.0.0.1"
				additional "{{ .Var.target }} 60 IN A 10.0.0.2"
				authority "example. 60 IN NS {{ .Var.target }}"
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
			expectedExtra:  []string{"ns0.example. 60 IN A 10.0.0.2"},
			expectedNs:     []string{"example. 60 IN NS ns0.example."},
		},
		{
			name: "NoVars",
			config: `template IN TXT example. {
				answer "{{ .Name }} 60 IN TXT \"{{ len .Var }}\""
			}`,
			qname:          "a.example.",
			qtype:          dns.TypeTXT,
			expectedAnswer: []string{`a.example. 60 IN TXT "0"`},
		},
		{
			name: "PerTemplateScope",
			config: `template IN A example. {
				match ^a[.]example[.]$
				var v '1'
				answer "{{ .Name }} 60 IN A 10.0.0.{{ .Var.v }}"
			}
			template IN A example. {
				match ^b[.]example[.]$
				answer "{{ .Name }} 60 IN TXT \"{{ len .Var }}\""
			}`,
			qname:          "a.example.",
			expectedAnswer: []string{"a.example. 60 IN A 10.0.0.1"},
		},
		{
			name: "RuntimeError",
			config: `template IN A example. {
				var bad incidr('notanip', '10.0.0.0/8')
				answer "{{ .Name }} 60 IN A 10.0.0.1"
			}`,
			qname:        "a.example.",
			expectedCode: dns.RcodeServerFailure,
		},
		{
			name: "RuntimeErrorWithFallthrough",
			config: `template IN A example. {
				var bad incidr('notanip', '10.0.0.0/8')
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
				var bad incidr('notanip', '10.0.0.0/8')
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
			if err != nil && !tc.expectedErr {
				t.Fatalf("expected no error, got: %v", err)
			}
			if err == nil && tc.expectedErr {
				t.Fatalf("expected an error, got none")
			}
			if code != tc.expectedCode {
				t.Fatalf("expected rcode %v, got %v", tc.expectedCode, code)
			}
			if tc.expectedCode != dns.RcodeSuccess {
				return
			}

			verifySection(t, "answer", rec.Msg.Answer, tc.expectedAnswer)
			verifySection(t, "additional", rec.Msg.Extra, tc.expectedExtra)
			verifySection(t, "authority", rec.Msg.Ns, tc.expectedNs)
		})
	}
}

func TestVarMatch(t *testing.T) {
	c := caddy.NewTestController("dns", `template IN A example. {
		match ^(?P<n>[0-9]+)[.]example[.]$
		var num int(group('n'))
		var double num * 2
		var label 'x' + group('n')
		var flag num > 1
	}`)
	handler, err := templateParse(c)
	if err != nil {
		t.Fatalf("expected no config error, got: %v", err)
	}

	req := &dns.Msg{Question: []dns.Question{{Name: "2.example.", Qclass: dns.ClassINET, Qtype: dns.TypeA}}}
	state := requestFor(req)

	data, match, fthrough := handler.Templates[0].match(context.Background(), state)
	if !match {
		t.Fatalf("expected a match, fallthrough %v", fthrough)
	}

	expected := map[string]any{
		"num":    2,
		"double": 4,
		"label":  "x2",
		"flag":   true,
	}
	if len(data.Var) != len(expected) {
		t.Fatalf("expected %d variables, got %d: %v", len(expected), len(data.Var), data.Var)
	}
	for name, want := range expected {
		got, ok := data.Var[name]
		if !ok {
			t.Errorf("variable %q missing", name)
			continue
		}
		if got != want {
			t.Errorf("variable %q: expected %#v, got %#v", name, want, got)
		}
	}
}

func TestVarMatchError(t *testing.T) {
	c := caddy.NewTestController("dns", `template IN A example. {
		var bad incidr('notanip', '10.0.0.0/8')
		fallthrough
	}`)
	handler, err := templateParse(c)
	if err != nil {
		t.Fatalf("expected no config error, got: %v", err)
	}

	req := &dns.Msg{Question: []dns.Question{{Name: "a.example.", Qclass: dns.ClassINET, Qtype: dns.TypeA}}}
	_, match, fthrough := handler.Templates[0].match(context.Background(), requestFor(req))
	if match {
		t.Fatal("expected no match")
	}
	if fthrough {
		t.Fatal("expected no fallthrough")
	}
}

func requestFor(req *dns.Msg) request.Request {
	return request.Request{W: &test.ResponseWriter{}, Req: req}
}

func verifySection(t *testing.T, section string, rrs []dns.RR, expected []string) {
	t.Helper()
	if len(rrs) != len(expected) {
		t.Fatalf("expected %d %s records, got %d: %v", len(expected), section, len(rrs), rrs)
	}
	for i, e := range expected {
		want, err := dns.NewRR(e)
		if err != nil {
			t.Fatalf("could not parse expected %s record %q: %v", section, e, err)
		}
		if rrs[i].String() != want.String() {
			t.Errorf("%s record %d: expected %q, got %q", section, i, want.String(), rrs[i].String())
		}
	}
}
