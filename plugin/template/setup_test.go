package template

import (
	"testing"

	"github.com/mholt/caddy"
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
		{`template ANY ANY (?P<x>`, true},
		{
			`template ANY ANY {

			}`,
			true,
		},
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
		// examples
		{
			`template ANY A ip-(?P<a>[0-9]*)-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]com {
				answer "{{ .Name }} A {{ .Group.a }}.{{ .Group.b }}.{{ .Group.c }}.{{ .Grup.d }}."
			}`,
			false,
		},
		{
			`template IN ANY "[.](example[.]com[.]dc1[.]example[.]com[.])$" {
				rcode NXDOMAIN
				answer "{{ index .Match 1 }} 60 IN SOA a.{{ index .Match 1 }} b.{{ index .Match 1 }} (1 60 60 60 60)"
			}`,
			false,
		},
		{
			`template IN A ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$ {
				answer "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
    			}
			template IN MX ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$ {
				answer "{{ .Name }} 60 IN MX 10 {{ .Name }}"
				additional "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
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
