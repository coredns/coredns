package tsig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/test"

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
	secretsFile, cleanup, err := test.TempFile(".", `key "name.key." {
	secret "test-key";
};
key "name2.key." {
	secret "test-key2";
};`)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer cleanup()

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
			input:           "tsig {\n secrets " + secretsFile + "\n}",
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

func TestParseKeyFile(t *testing.T) {
	var reader = strings.NewReader(`key "foo" {
	algorithm hmac-sha256;
	secret "36eowrtmxceNA3T5AdE+JNUOWFCw3amtcyHACnrDVgQ=";
};
key "bar" {
	algorithm hmac-sha256;
	secret "X28hl0BOfAL5G0jsmJWSacrwn7YRm2f6U5brnzwWEus=";
};
key "baz" {
	secret "BycDPXSx/5YCD44Q4g5Nd2QNxNRDKwWTXddrU/zpIQM=";
};`)

	secrets, err := parseKeyFile(reader)
	if err != nil {
		t.Fatalf("Unexpected error: %q", err)
	}
	expectedSecrets := map[string]string{
		"foo.": "36eowrtmxceNA3T5AdE+JNUOWFCw3amtcyHACnrDVgQ=",
		"bar.": "X28hl0BOfAL5G0jsmJWSacrwn7YRm2f6U5brnzwWEus=",
		"baz.": "BycDPXSx/5YCD44Q4g5Nd2QNxNRDKwWTXddrU/zpIQM=",
	}

	if len(secrets) != len(expectedSecrets) {
		t.Fatalf("result has %d keys. expected %d", len(secrets), len(expectedSecrets))
	}

	for k, sec := range secrets {
		expectedSec, ok := expectedSecrets[k]
		if !ok {
			t.Errorf("unexpected key in result. %q", k)
			continue
		}
		if sec != expectedSec {
			t.Errorf("incorrect secret in result for key %q. expected %q got %q ", k, expectedSec, sec)
		}
	}
}

func TestParseKeyFileErrors(t *testing.T) {
	tests := []struct {
		in  string
		err string
	}{
		{in: `key {`, err: "expected key name \"key {\""},
		{in: `foo "key" {`, err: "unexpected token \"foo\""},
		{
			in: `key "foo" {
		secret "36eowrtmxceNA3T5AdE+JNUOWFCw3amtcyHACnrDVgQ=";
	};
		key "foo" {
		secret "X28hl0BOfAL5G0jsmJWSacrwn7YRm2f6U5brnzwWEus=";
	}; `,
			err: "key \"foo.\" redefined",
		},
		{in: `key "foo" {
	schmalgorithm hmac-sha256;`,
			err: "unexpected token \"schmalgorithm\"",
		},
		{
			in: `key "foo" {
	schmecret "36eowrtmxceNA3T5AdE+JNUOWFCw3amtcyHACnrDVgQ=";`,
			err: "unexpected token \"schmecret\"",
		},
		{
			in: `key "foo" {
	secret`,
			err: "expected secret key \"\\tsecret\"",
		},
		{
			in: `key "foo" {
	secret ;`,
			err: "expected secret key \"\\tsecret ;\"",
		},
		{
			in: `key "foo" {
	};`,
			err: "expected secret for key \"foo.\"",
		},
	}
	for i, testcase := range tests {
		_, err := parseKeyFile(strings.NewReader(testcase.in))
		if err == nil {
			t.Errorf("Test %d: expected error, got no error", i)
			continue
		}
		if err.Error() != testcase.err {
			t.Errorf("Test %d: Expected error: %q, got %q", i, testcase.err, err.Error())
		}
	}
}
