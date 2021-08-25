package acme

import (
	"reflect"
	"testing"

	"github.com/caddyserver/certmagic"
	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		shouldErr    bool
		acmeTemplate certmagic.ACMEManager
		zone         string
	}{
		{
			"Correct Config with only DNS challenge",
			`acme test.domain`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    true,
				DisableTLSALPNChallenge: true,
				AltHTTPPort:             80,
				AltTLSALPNPort:          443,
			},
			"test.domain",
		},
		{
			"Correct Config with correct challenge",
			`acme test.domain {
				http01 89
				tlsalpn 90
			}`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    false,
				DisableTLSALPNChallenge: false,
				AltHTTPPort:             89,
				AltTLSALPNPort:          90,
			},
			"test.domain",
		},
		{
			"Correct Config with default http01 port",
			`acme test.domain {
				tlsalpn 90
			}`,
			false,
			certmagic.ACMEManager{
				DisableHTTPChallenge:    false,
				DisableTLSALPNChallenge: false,
				AltHTTPPort:             80,
				AltTLSALPNPort:          90,
			},
			"test.domain",
		},
		{
			"Missing domain",
			`acme {
				http01 hello
			}`,
			true,
			certmagic.ACMEManager{},
			"",
		},
		{
			"Invalid port",
			`acme test.domain {
				http01 hello
			}`,
			true,
			certmagic.ACMEManager{},
			"test.domain",
		},
		{
			"Invalid challenge",
			`acme test.domain {
				invalid_challenge
			`,
			true,
			certmagic.ACMEManager{},
			"test.domain",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := caddy.NewTestController("acme", test.input)
			acmeTemplate, zone, err := parseACME(c)
			if (err != nil) != test.shouldErr {
				t.Errorf("Error: setup() error = %v, shouldErr %v", err, test.shouldErr)
				if !test.shouldErr && err != nil && reflect.DeepEqual(test.acmeTemplate, acmeTemplate) && test.zone == zone {
					t.Errorf("Error: AcmeTemplate %+v Zone %s is not configured as it should be %+v", acmeTemplate, zone, test.acmeTemplate)
				}
			}
		})
	}
}
