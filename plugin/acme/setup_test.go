package acme

import (
	"testing"

	"github.com/caddyserver/certmagic"
	"github.com/coredns/caddy"
)

func compareAcmeTemplate(a, b certmagic.ACMEManager) bool {
	return a.DisableHTTPChallenge == b.DisableHTTPChallenge && a.AltTLSALPNPort == b.AltTLSALPNPort && a.AltHTTPPort == b.AltHTTPPort && a.DisableTLSALPNChallenge == b.DisableTLSALPNChallenge
}
func TestSetup(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		shouldErr    bool
		acmeTemplate certmagic.ACMEManager
		zoneName     string
	}{
		{
			"Correct Config with only DNS challenge",
			`acme {
				domain test.domain
			}`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    true,
				DisableTLSALPNChallenge: true,
				AltHTTPPort:             0,
				AltTLSALPNPort:          0,
			},
			"test.domain",
		},
		{
			"Correct Config with correct challenge",
			`acme {
				domain test.domain
				challenge http port 89
				challenge tlsalpn port 8081
			}`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    false,
				DisableTLSALPNChallenge: false,
				AltHTTPPort:             89,
				AltTLSALPNPort:          8081,
			},
			"test.domain",
		},
		{
			"Correct Config with tlsalpn port",
			`acme {
				domain test.domain
				challenge tlsalpn port 90
			}`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    true,
				DisableTLSALPNChallenge: false,
				AltHTTPPort:             0,
				AltTLSALPNPort:          90,
			},
			"test.domain",
		},
		{
			"Missing domain",
			`acme {
				http hello
			}`,
			true,
			certmagic.ACMEManager{},
			"",
		},
		{
			"Invalid port",
			`acme {
				domain test.domain
				challenge http port hello
			}`,
			true,
			certmagic.ACMEManager{},
			"test.domain",
		},
		{
			"Invalid challenge",
			`acme {
				domain test.domain
				invalid_challenge
			`,
			true,
			certmagic.ACMEManager{},
			"test.domain",
		},
		{
			"Invalid challenge format",
			`acme {
				domain test.domain
				challenge http 90
			`,
			true,
			certmagic.ACMEManager{},
			"test.domain",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := caddy.NewTestController("acme", test.input)
			acmeTemplate, zoneName, err := parseACME(c)
			if (err != nil) != test.shouldErr {
				t.Errorf("Error: setup() error = %v, shouldErr %v", err, test.shouldErr)
			} else {
				if !test.shouldErr {
					if !compareAcmeTemplate(test.acmeTemplate, acmeTemplate) {
						t.Errorf("Error: AcmeTemplate %+v is not configured as it should be %+v", acmeTemplate, test.acmeTemplate)
					}
					if test.zoneName != zoneName {
						t.Errorf("Error: Expected zone %s but got %+v", test.zoneName, zoneName)
					}
				}
			}
		})
	}
}
