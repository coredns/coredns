package kubernetes

import (
	"context"
	"slices"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	testhelper "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

func TestAutoPath(t *testing.T) {
	// Set up a Kubernetes object for testing
	defaultZone := "interwebs.test."
	k := New([]string{defaultZone})
	k.autoPathSearch = []string{"custom."}
	k.APIConn = &APIConnServiceTest{}
	k.podMode = podModeVerified
	k.opts.initPodCache = true

	type autopathTest struct {
		qname      string
		searchpath []string
		ip         string
		zone       string
	}
	tests := []autopathTest{
		{
			// Cluster IP Service FQDN - first query
			qname: "svc1.testns.svc.interwebs.test.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Cluster IP Service name only or service inside custom domain - first query
			qname: "svc1.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// External service - first query
			qname: "example.com.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// External service matching zone "." - first query
			qname: "example.com.testns.svc.",
			zone:  ".",
			searchpath: []string{
				"testns.svc.",
				"svc.",
				".",
				"custom.",
				"",
			},
		},
		{
			// External service matching zone "." - first query - host pod
			qname: "example.com.testns.svc.",
			zone:  ".",
			ip:    "10.16.0.1",
			searchpath: []string{
				"testns.svc.",
				"svc.",
				".",
				"custom.",
				"",
			},
		},
		{
			// External service - first query - host pod
			qname: "example.com.other.svc.interwebs.test.",
			ip:    "10.16.0.1",
			searchpath: []string{
				"other.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Zone apex query - host pod
			qname: "interwebs.test.",
			ip:    "10.16.0.1",
		},
		{
			// Query without namespace - host pod
			qname: "svc.interwebs.test.",
			ip:    "10.16.0.1",
		},
		{
			// Query without the Kubernetes service label - host pod
			qname: "example.other.not-svc.interwebs.test.",
			ip:    "10.16.0.1",
		},
		{
			// External service - second query
			qname: "example.com.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with other namespace in second query - normal pod
			qname: "example.other.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with testns namespace in second query - normal pod
			qname: "example.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with other namespace in second query - host pod
			qname: "example.other.svc.interwebs.test.",
			ip:    "10.16.0.1",
			searchpath: []string{
				"other.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
	}

	for _, test := range tests {
		writer := &testhelper.ResponseWriter{}
		if test.ip != "" {
			writer.RemoteIP = test.ip
		}
		zone := "interwebs.test."
		if test.zone != "" {
			zone = test.zone
			k.Zones[0] = test.zone
		}
		state := request.Request{
			Req:  &dns.Msg{Question: []dns.Question{{Name: test.qname, Qtype: dns.TypeA}}},
			Zone: zone, // must match from k.Zones[0]
			W:    writer,
		}
		searchpath := k.AutoPath(state)
		if !slices.Equal(searchpath, test.searchpath) {
			t.Errorf("Error in query %s: expected searchpath %v, but got %v", test.qname, test.searchpath, searchpath)
		}
		if test.zone != "" {
			k.Zones[0] = defaultZone
		}
	}
}

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
	state := request.Request{W: &testhelper.ResponseWriter{}, Req: req}

	b.ReportAllocs()

	for b.Loop() {
		result := k.AutoPath(state)
		_ = result
	}
}
