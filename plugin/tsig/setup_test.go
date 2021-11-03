package tsig

import (
	"fmt"
	"testing"

	"github.com/coredns/caddy"

	"github.com/miekg/dns"
)

func TestParse(t *testing.T) {

	secrets := map[string]string{
		"name.key.":  "test-key",
		"name2.key.": "test-key-2",
	}
	secretConfig := ""
	for k, s := range secrets {
		secretConfig += fmt.Sprintf("secret %s %s\n", k, s)
	}
	tests := []struct {
		input           string
		shouldErr       bool
		expectedZones   []string
		expectedQTypes  qTypes
		expectedSecrets map[string]string
		expectedAll     bool
	}{
		{
			input:           "tsig {\n " + secretConfig + "}",
			expectedZones:   []string{"."},
			expectedQTypes:  defaultQTypes,
			expectedSecrets: secrets,
		},
		{
			input:           "tsig example.com {\n " + secretConfig + "}",
			expectedZones:   []string{"example.com."},
			expectedQTypes:  defaultQTypes,
			expectedSecrets: secrets,
		},
		{
			input:           "tsig {\n " + secretConfig + " require all \n}",
			expectedZones:   []string{"."},
			expectedQTypes:  qTypes{},
			expectedAll:     true,
			expectedSecrets: secrets,
		},
		{
			input:           "tsig {\n " + secretConfig + " require none \n}",
			expectedZones:   []string{"."},
			expectedQTypes:  qTypes{},
			expectedAll:     false,
			expectedSecrets: secrets,
		},
		{
			input:           "tsig {\n " + secretConfig + " \n require A AAAA \n}",
			expectedZones:   []string{"."},
			expectedQTypes:  qTypes{dns.TypeA: {}, dns.TypeAAAA: {}},
			expectedSecrets: secrets,
		},
		{
			input:     "tsig {\n blah \n}",
			shouldErr: true,
		},
		{
			input:     "tsig {\n secret name. too many parameters \n}",
			shouldErr: true,
		},
		{
			input:     "tsig {\n require \n}",
			shouldErr: true,
		},
		{
			input:     "tsig {\n require invalid-qtype \n}",
			shouldErr: true,
		},
	}

	serverBlockKeys := []string{"."}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		c.ServerBlockKeys = serverBlockKeys
		ts, err := parse(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error.", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}

		if test.shouldErr {
			continue
		}

		if len(test.expectedZones) != len(ts.Zones) {
			t.Fatalf("Test %d expected zones '%v', but got '%v'.", i, test.expectedZones, ts.Zones)
		}
		for j := range test.expectedZones {
			if test.expectedZones[j] != ts.Zones[j] {
				t.Errorf("Test %d expected zones '%v', but got '%v'.", i, test.expectedZones, ts.Zones)
				break
			}
		}

		if test.expectedAll != ts.all {
			t.Errorf("Test %d expected require all to be '%v', but got '%v'.", i, test.expectedAll, ts.all)
		}

		if len(test.expectedQTypes) != len(ts.types) {
			t.Fatalf("Test %d expected required types '%v', but got '%v'.", i, test.expectedQTypes, ts.types)
		}
		for qt := range test.expectedQTypes {
			if _, ok := ts.types[qt]; !ok {
				t.Errorf("Test %d required types '%v', but got '%v'.", i, test.expectedQTypes, ts.types)
				break
			}
		}

		if len(test.expectedSecrets) != len(ts.secrets) {
			t.Fatalf("Test %d expected secrets '%v', but got '%v'.", i, test.expectedSecrets, ts.secrets)
		}
		for qt := range test.expectedSecrets {
			secret, ok := ts.secrets[qt]
			if !ok {
				t.Errorf("Test %d required secrets '%v', but got '%v'.", i, test.expectedSecrets, ts.secrets)
				break
			}
			if secret != ts.secrets[qt] {
				t.Errorf("Test %d required secrets '%v', but got '%v'.", i, test.expectedSecrets, ts.secrets)
				break
			}
		}

	}
}
