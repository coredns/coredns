package metadata

import (
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/coredns/caddy"

	"github.com/miekg/dns"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		zones     []string
		shouldErr bool
	}{
		{"metadata", []string{}, false},
		{"metadata example.com.", []string{"example.com."}, false},
		{"metadata example.com. net.", []string{"example.com.", "net."}, false},

		{"metadata example.com. { some_param }", []string{}, true},
		{"metadata\nmetadata", []string{}, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Setup call expected error but found none for input %s", i, test.input)
		}

		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Setup call expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}
	}
}

func TestSetupHealth(t *testing.T) {
	tests := []struct {
		input     string
		zones     []string
		shouldErr bool
	}{
		{"metadata", []string{}, false},
		{"metadata example.com.", []string{"example.com."}, false},
		{"metadata example.com. net.", []string{"example.com.", "net."}, false},

		{"metadata example.com. { some_param }", []string{}, true},
		{"metadata\nmetadata", []string{}, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		m, err := metadataParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}

		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}

		if !test.shouldErr && err == nil {
			if !reflect.DeepEqual(test.zones, m.Zones) {
				t.Errorf("Test %d: Expected zones %s. Zones were: %v", i, test.zones, m.Zones)
			}
		}
	}
}

func Test_parseMap(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedErr string
		expected    *Map
	}{
		{
			name: "1_metadata",
			input: `metadata {
						map {{.Type}} key1 {
							A         val11
							AAAA      "val 21"
							default   val31
						}
					}`,
			expected: &Map{
				Source: template.Must(template.New("{{.Type}}").Parse("{{.Type}}")),
				Mapping: map[string][]KeyValue{
					"A":        {{Key: "key1", Value: "val11"}},
					"AAAA":     {{Key: "key1", Value: "val 21"}},
					defaultKey: {{Key: "key1", Value: "val31"}},
				},
			},
		},
		{
			name: "2_metadata",
			input: `metadata {
						map {{.Type}} key1     key2 {
							A         val11    val12
							AAAA      "val 21" "val 22"
							default   val31    val32
						}
					}`,
			expected: &Map{
				Source: template.Must(template.New("{{.Type}}").Parse("{{.Type}}")),
				Mapping: map[string][]KeyValue{
					"A":        {{Key: "key1", Value: "val11"}, {Key: "key2", Value: "val12"}},
					"AAAA":     {{Key: "key1", Value: "val 21"}, {Key: "key2", Value: "val 22"}},
					defaultKey: {{Key: "key1", Value: "val31"}, {Key: "key2", Value: "val32"}},
				},
			},
		},
		{
			name: "2_sources",
			input: `metadata {
						map "{{.Type}} {{.IP}}" key1    key2 {
							"A 127.0.0.1"      val11    val12
							"AAAA 127.0.0.1"   "val 21" "val 22"
							default            val31    val32
						}
					}`,
			expected: &Map{
				Source: template.Must(template.New("{{.Type}} {{.IP}}").Parse("{{.Type}} {{.IP}}")),
				Mapping: map[string][]KeyValue{
					"A 127.0.0.1":    {{Key: "key1", Value: "val11"}, {Key: "key2", Value: "val12"}},
					"AAAA 127.0.0.1": {{Key: "key1", Value: "val 21"}, {Key: "key2", Value: "val 22"}},
					defaultKey:       {{Key: "key1", Value: "val31"}, {Key: "key2", Value: "val32"}},
				},
			},
		},
		{
			name: "no_default",
			input: `metadata {
						map {{.Type}} key1     key2 {
							A         val11    val12
							AAAA      "val 21" "val 22"
						}
					}`,
			expected: &Map{
				Source: template.Must(template.New("{{.Type}}").Parse("{{.Type}}")),
				Mapping: map[string][]KeyValue{
					"A":    {{Key: "key1", Value: "val11"}, {Key: "key2", Value: "val12"}},
					"AAAA": {{Key: "key1", Value: "val 21"}, {Key: "key2", Value: "val 22"}},
				},
			},
		},
		{
			name: "default_rcode",
			input: `metadata {
						map {{.Type}} key1     key2 {
							A         val11    val12
							AAAA      "val 21" "val 22"
							default   SERVFAIL
						}
					}`,
			expected: &Map{
				Source: template.Must(template.New("{{.Type}}").Parse("{{.Type}}")),
				Mapping: map[string][]KeyValue{
					"A":    {{Key: "key1", Value: "val11"}, {Key: "key2", Value: "val12"}},
					"AAAA": {{Key: "key1", Value: "val 21"}, {Key: "key2", Value: "val 22"}},
				},
				RcodeOnMiss: dns.RcodeServerFailure,
			},
		},
		{
			name: "invalid_source",
			input: `metadata {
						map {{end}} key1     key2 {
							A       val11    val12
						}
					}`,
			expectedErr: "unexpected {{end}}",
		},
		{
			name: "values_more_than_labels",
			input: `metadata {
						map {{.Type}} key1  key2 {
							A         val11 val12 val13
						}
					}`,
			expectedErr: "number of values must match labels",
		},
		{
			name: "values_less_than_labels",
			input: `metadata {
						map {{.Type}} key1  key2 {
							A         val11
						}
					}`,
			expectedErr: "number of values must match labels",
		},
		{
			name: "default_values_less_than_labels",
			input: `metadata {
						map {{.Type}} key1  key2 {
							A         val11 val12
							default   val31
						}
					}`,
			expectedErr: "number of values must match labels",
		},
		{
			name: "no_labels",
			input: `metadata {
						map {{.Type}} {
							A         val11
						}
					}`,
			expectedErr: "missing label(s)",
		},
		{
			name: "no_values",
			input: `metadata {
						map {{.Type}} key1  key2 {
							A
						}
					}`,
			expectedErr: "number of values must match labels",
		},
		{
			name: "no_rows",
			input: `metadata {
						map {{.Type}} key1 key2 {
						}
					}`,
			expectedErr: "missing value(s)",
		},
		{
			name: "no_source",
			input: `metadata {
						map
					}`,
			expectedErr: "missing source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := metadataParse(caddy.NewTestController("dns", test.input))

			if test.expectedErr != "" && err == nil {
				t.Errorf("Expected error but found %s for input '%s'", err, test.input)
			}

			if err != nil {
				if test.expectedErr == "" {
					t.Errorf("Expected no error but found one for input '%s'. Error was: %v", test.input, err)
				}
				if !strings.Contains(err.Error(), test.expectedErr) {
					t.Errorf("Expected error: %v, found error: %v, input: '%s'", test.expectedErr, err, test.input)
				}
			}

			if v != nil && !reflect.DeepEqual(v.Map, test.expected) {
				t.Errorf("Expected metamap: %v but got: %v, input: '%s'", test.expected, v, test.input)
			}
		})
	}
}
