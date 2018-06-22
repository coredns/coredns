package metadata

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// testMetadater implements fake Metadaters (plugins which inmplement Metadater interface)
type testMetadater struct {
	key   interface{}
	value interface{}
}

func (m testMetadater) Metadata(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (context.Context, error) {
	return context.WithValue(ctx, m.key, m.value), nil
}

// testHandler implements plugin.Handler
type testHandler struct{ ctx context.Context }

func (m *testHandler) Name() string { return "testHandler" }

func (m *testHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m.ctx = ctx
	return 0, nil
}

func TestMetadataServDns(t *testing.T) {
	expectedMetadata := []testMetadater{
		{"testkey", "testvalue"},
		{500, 795},
	}
	// Create fake metadaters based on expectedMetadata
	metadaters := []Metadater{}
	for _, e := range expectedMetadata {
		metadaters = append(metadaters, e)
	}
	// Fake handler which stores the resulting context
	next := &testHandler{}

	metadata := Metadata{
		Metadaters: metadaters,
		Next:       next,
	}
	metadata.ServeDNS(context.TODO(), &test.ResponseWriter{}, new(dns.Msg))

	// Verify that next plugin can find metadata in context from all metadaters
	for _, expected := range expectedMetadata {
		metadataVal := next.ctx.Value(expected.key)
		if metadataVal != expected.value {
			t.Errorf("Expected value %v, but got %v", expected.value, metadataVal)
		}
	}
}
