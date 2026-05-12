package kubernetes

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

// Mock data for benchmarks
var (
	mockPod = &object.Pod{
		Namespace: "test-namespace",
	}
	mockAutoPathSearch = []string{"example.com", "internal.example.com", "cluster.local"}
)

// Mock API connector for testing
type mockAPIConnector struct{}

func (m *mockAPIConnector) PodIndex(_ip string) []*object.Pod {
	return []*object.Pod{mockPod}
}

// Minimal implementation of other required methods
func (m *mockAPIConnector) ServiceList() []*object.Service                      { return nil }
func (m *mockAPIConnector) EndpointsList() []*object.Endpoints                  { return nil }
func (m *mockAPIConnector) ServiceImportList() []*object.ServiceImport          { return nil }
func (m *mockAPIConnector) SvcIndex(_s string) []*object.Service                { return nil }
func (m *mockAPIConnector) SvcIndexReverse(_s string) []*object.Service         { return nil }
func (m *mockAPIConnector) SvcExtIndexReverse(_s string) []*object.Service      { return nil }
func (m *mockAPIConnector) SvcImportIndex(_s string) []*object.ServiceImport    { return nil }
func (m *mockAPIConnector) EpIndex(_s string) []*object.Endpoints               { return nil }
func (m *mockAPIConnector) EpIndexReverse(_s string) []*object.Endpoints        { return nil }
func (m *mockAPIConnector) McEpIndex(_s string) []*object.MultiClusterEndpoints { return nil }
func (m *mockAPIConnector) GetNodeByName(_ctx context.Context, _name string) (*api.Node, error) {
	return nil, nil
}
func (m *mockAPIConnector) GetNamespaceByName(_name string) (*object.Namespace, error) {
	return nil, nil
}
func (m *mockAPIConnector) Run()                        {}
func (m *mockAPIConnector) HasSynced() bool             { return true }
func (m *mockAPIConnector) Stop() error                 { return nil }
func (m *mockAPIConnector) Modified(ModifiedMode) int64 { return 0 }

func BenchmarkAutoPath(b *testing.B) {
	k := &Kubernetes{
		Zones:          []string{"cluster.local."},
		autoPathSearch: mockAutoPathSearch,
		podMode:        podModeVerified,
		opts: dnsControlOpts{
			initPodCache: true,
		},
		APIConn: &mockAPIConnector{},
	}

	// Create a mock DNS request
	req := &dns.Msg{}
	req.SetQuestion("test.cluster.local.", dns.TypeA)

	// Create a request state with a mock ResponseWriter
	state := request.Request{W: &test.ResponseWriter{}, Req: req}

	b.ReportAllocs()

	for b.Loop() {
		result := k.AutoPath(state)
		_ = result
	}
}
