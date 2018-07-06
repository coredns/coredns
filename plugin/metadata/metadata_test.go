package metadata

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type testProvider map[string]Func

func (tp testProvider) Metadata(ctx context.Context, state request.Request) context.Context {
	for k, v := range tp {
		SetValueFunc(ctx, k, v)
	}
	return ctx
}

type testHandler struct{ ctx context.Context }

func (m *testHandler) Name() string { return "test" }

func (m *testHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m.ctx = ctx
	return 0, nil
}

func TestMetadataServeDNS(t *testing.T) {
	expectedMetadata := []testProvider{
		{"test/key1": func() string { return "testvalue1" }},
		{"test/key2": func() string { return "two" }, "test/key3": func() string { return "testvalue3" }},
	}
	// Create fake Providers based on expectedMetadata
	providers := []Provider{}
	for _, e := range expectedMetadata {
		providers = append(providers, e)
	}

	next := &testHandler{} // fake handler which stores the resulting context
	m := Metadata{
		Zones:     []string{"."},
		Providers: providers,
		Next:      next,
	}

	ctx := context.TODO()
	if IsMetadataSet(ctx) {
		t.Errorf("Context provide Metadata information whereas metadata is not yet activated")
	}
	m.ServeDNS(ctx, &test.ResponseWriter{}, new(dns.Msg))
	nctx := next.ctx

	if !IsMetadataSet(nctx) {
		t.Errorf("Context does not provide Metadata information")
	}

	for _, expected := range expectedMetadata {
		for label, expVal := range expected {
			if !IsLabel(label) {
				t.Errorf("Expected label %s is not considered a valid label", label)
			}
			val := ValueFunc(nctx, label)
			if val() != expVal() {
				t.Errorf("Expected value %s for %s, but got %s", expVal(), label, val())
			}
		}
	}
}

func TestLabelFormat(t *testing.T) {
	labels := []struct {
		label   string
		isValid bool
	}{
		{"plugin/LABEL", true},
		{"LABEL", false},
		{"plugin.LABEL", false},
		{"/NO-PLUGIN-ACCEPTED", true},
		{"ONLY-PLUGIN-ACCEPTED/", true},
		{"PLUGIN/LABEL/SUB-LABEL", true},
	}

	for _, test := range labels {
		if IsLabel(test.label) != test.isValid {
			t.Errorf("Label %v is expected to have this validaty : %v - and has the opposite", test.label, test.isValid)
		}
	}
}
