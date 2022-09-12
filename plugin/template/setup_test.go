package template

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `template ANY ANY {
		rcode
	}`)
	err := setupTemplate(c)
	if err == nil {
		t.Errorf("Expected setupTemplate to fail on broken template, got no error")
	}
	c = caddy.NewTestController("dns", `template ANY ANY {
		rcode NXDOMAIN
	}`)
	err = setupTemplate(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
}

func TestSetupTemplateFunction(t *testing.T) {
	c := caddy.NewTestController("dns", `template IN A example {
		match "^ip0a(?P<b>[a-f0-9]{2})(?P<c>[a-f0-9]{2})(?P<d>[a-f0-9]{2})[.]example[.]$"
		answer "{{ .Name }} 60 IN A 10.{{ parseInt .Group.b 16 8 }}.{{ parseInt .Group.c 16 8 }}.{{ parseInt .Group.d 16 8 }}"
	}`)

	c.ServerBlockKeys = []string{".:8053"}

	templates, err := templateParse(c)

	if err != nil {
		t.Fatalf("Unexpected error parsing template %q", err)
	}

	r := &dns.Msg{Question: []dns.Question{{Name: "ip0a5f0c09.example.", Qclass: dns.ClassINET, Qtype: dns.TypeA}}}
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	_, err = templates.ServeDNS(context.TODO(), rec, r)
	if err != nil {
		t.Fatalf("Unexpected error serving DNS %q", err)
	}
	if rec.Msg.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %v", len(rec.Msg.Answer))
	}
	if x := rec.Msg.Answer[0].(*dns.A).A.String(); x != "10.95.12.9" {
		t.Fatalf("Failed to get address for A record, expected 10.95.12.9 got %s", x)
	}
}

func TestSetupParse(t *testing.T) {
	serverBlockKeys := []string{"domain.com.:8053", "dynamic.domain.com.:8053"}

	tests := []struct {
		inputFileRules string
		shouldErr      bool
	}{
		// parse errors
		{`template`, true},
		{`template X`, true},
		{`template ANY`, true},
		{`template ANY X`, true},
		{
			`template ANY ANY .* {
				notavailable
			}`,
			true,
		},
		{
			`template ANY ANY {
				answer
			}`,
			true,
		},
		{
			`template ANY ANY {
				additional
			}`,
			true,
		},
		{
			`template ANY ANY {
				rcode
			}`,
			true,
		},
		{
			`template ANY ANY {
				rcode UNDEFINED
			}`,
			true,
		},
		{
			`template ANY ANY {
				answer	"{{"
			}`,
			true,
		},
		{
			`template ANY ANY {
				additional "{{"
			}`,
			true,
		},
		{
			`template ANY ANY {
				authority "{{"
			}`,
			true,
		},
		{
			`template ANY ANY {
				answer "{{ notAFunction }}"
			}`,
			true,
		},
		{
			`template ANY ANY {
				answer "{{ parseInt }}"
				additional "{{ parseInt }}"
				authority "{{ parseInt }}"
			}`,
			false,
		},
		// examples
		{`template ANY ANY (?P<x>`, false},
		{
			`template ANY ANY {

			}`,
			false,
		},
		{
			`template ANY A example.com {
				match ip-(?P<a>[0-9]*)-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]com
				answer "{{ .Name }} A {{ .Group.a }}.{{ .Group.b }}.{{ .Group.c }}.{{ .Grup.d }}."
				fallthrough
			}`,
			false,
		},
		{
			`template ANY AAAA example.com {
				match ip-(?P<a>[0-9]*)-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]com
				authority "example.com 60 IN SOA ns.example.com hostmaster.example.com (1 60 60 60 60)"
				fallthrough
			}`,
			false,
		},
		{
			`template IN ANY example.com {
				match "[.](example[.]com[.]dc1[.]example[.]com[.])$"
				rcode NXDOMAIN
				authority "{{ index .Match 1 }} 60 IN SOA ns.{{ index .Match 1 }} hostmaster.example.com (1 60 60 60 60)"
				fallthrough example.com
			}`,
			false,
		},
		{
			`template IN A example {
				match ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$
				answer "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
			}
			template IN MX example. {
				match ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$
				answer "{{ .Name }} 60 IN MX 10 {{ .Name }}"
				additional "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
			}`,
			false,
		},
		{
			`template IN A example {
				match ^ip0a(?P<b>[a-f0-9]{2})(?P<c>[a-f0-9]{2})(?P<d>[a-f0-9]{2})[.]example[.]$
				answer "{{ .Name }} 3600 IN A 10.{{ parseInt .Group.b 16 8 }}.{{ parseInt .Group.c 16 8 }}.{{ parseInt .Group.d 16 8 }}"
			}`,
			false,
		},
		{
			`template IN MX example {
					match ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$
					answer "{{ .Name }} 60 IN MX 10 {{ .Name }}"
					additional "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
					authority  "example. 60 IN NS ns0.example."
					authority  "example. 60 IN NS ns1.example."
					additional "ns0.example. 60 IN A 203.0.113.8"
					additional "ns1.example. 60 IN A 198.51.100.8"
				}`,
			false,
		},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputFileRules)
		c.ServerBlockKeys = serverBlockKeys
		templates, err := templateParse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error\n---\n%s\n---\n%v", i, test.inputFileRules, templates)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}
	}
}
