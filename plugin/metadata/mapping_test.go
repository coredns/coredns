package metadata

import (
	"context"
	"testing"
	"text/template"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestMetadata_Metadata(t *testing.T) {
	tests := []struct {
		name     string
		input    request.Request
		m        *Metadata
		expected map[string]string
	}{
		{
			name: "2_metadata",
			input: request.Request{
				W: &test.ResponseWriter{RemoteIP: "1.1.1.1"},
			},
			m: &Metadata{
				Map: &Map{
					Source: template.Must(template.New("{{.IP}}").Parse("{{.IP}}")),
					Mapping: map[string][]KeyValue{
						"1.1.1.1": {KeyValue{Key: "foo", Value: "bar"}, KeyValue{Key: "bar", Value: "baz"}},
					},
				},
			},
			expected: map[string]string{
				"metadata/foo": "bar",
				"metadata/bar": "baz",
			},
		},
		{
			name: "default",
			input: request.Request{
				W: &test.ResponseWriter{RemoteIP: "2.2.2.2"},
			},
			m: &Metadata{
				Map: &Map{
					Source: template.Must(template.New("{{.IP}}").Parse("{{.IP}}")),
					Mapping: map[string][]KeyValue{
						"1.1.1.1":  {KeyValue{Key: "foo", Value: "bar"}},
						defaultKey: {KeyValue{Key: "bar", Value: "baz"}},
					},
				},
			},
			expected: map[string]string{
				"metadata/bar": "baz",
			},
		},
		{
			name: "default_error",
			input: request.Request{
				W: &test.ResponseWriter{RemoteIP: "2.2.2.2"},
			},
			m: &Metadata{
				Map: &Map{
					Source: template.Must(template.New("{{.IP}}").Parse("{{.IP}}")),
					Mapping: map[string][]KeyValue{
						"1.1.1.1": {KeyValue{Key: "foo", Value: "bar"}},
					},
					RcodeOnMiss: dns.RcodeServerFailure,
				},
			},
			expected: map[string]string{
				errorLabel: "no values found for {{.IP}} '2.2.2.2'",
			},
		},
		{
			name: "no_default",
			input: request.Request{
				W: &test.ResponseWriter{RemoteIP: "2.2.2.2"},
			},
			m: &Metadata{
				Map: &Map{
					Source: template.Must(template.New("{{.IP}}").Parse("{{.IP}}")),
					Mapping: map[string][]KeyValue{
						"1.1.1.1": {KeyValue{Key: "foo", Value: "bar"}},
					},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "no_mapping",
			input: request.Request{
				W: &test.ResponseWriter{RemoteIP: "2.2.2.2"},
			},
			m: &Metadata{
				Map: nil,
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextWithMetadata(context.Background())

			ctx = tt.m.Metadata(ctx, tt.input)

			allValues := ValueFuncs(ctx)
			if len(allValues) != len(tt.expected) {
				t.Errorf("expected %d metadata values, got %d", len(tt.expected), len(allValues))
			}

			for label, f := range allValues {
				expected, ok := tt.expected[label]
				if !ok {
					t.Errorf("unexpected metadata '%s'", label)
				}
				if expected != f() {
					t.Errorf("got unexpected Value for Key '%s': expected '%s' got '%s'", label, expected, f())
				}
			}
		})
	}
}

func TestMetadata_ServeDNS(t *testing.T) {
	ctx := context.Background()
	m := Metadata{
		Next: test.NextHandler(dns.RcodeSuccess, nil),
		Map: &Map{
			RcodeOnMiss: dns.RcodeServerFailure,
		},
	}

	// no error in metadata
	rcode, err := m.ServeDNS(ctx, &test.ResponseWriter{}, new(dns.Msg))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if rcode != dns.RcodeSuccess {
		t.Errorf("Expected Rcode to be %v, got %v", dns.RcodeSuccess, rcode)
	}

	// error in metadata
	expectedErr := "test error"

	ctx = ContextWithMetadata(ctx)
	setError(ctx, expectedErr)

	rcode, err = m.ServeDNS(ctx, &test.ResponseWriter{}, new(dns.Msg))
	if err == nil || err.Error() != expectedErr {
		t.Errorf("Expected %v, got %v", expectedErr, err)
	}
	if rcode != dns.RcodeServerFailure {
		t.Errorf("Expected Rcode to be %v, got %v", dns.RcodeServerFailure, rcode)
	}
}
