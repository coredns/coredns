package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/miekg/dns"

	api "k8s.io/api/core/v1"
)

type APIConnTest struct{}

func (APIConnTest) HasSynced() bool                          { return true }
func (APIConnTest) Run()                                     { return }
func (APIConnTest) Stop() error                              { return nil }
func (APIConnTest) PodIndex(string) []*object.Pod            { return nil }
func (APIConnTest) SvcIndexReverse(string) []*object.Service { return nil }
func (APIConnTest) EpIndex(string) []*object.Endpoints       { return nil }
func (APIConnTest) EndpointsList() []*object.Endpoints       { return nil }
func (APIConnTest) Modified() int64                          { return 0 }

func (a APIConnTest) SvcIndex(s string) []*object.Service {
	switch s {
	case "dns-service.kube-system":
		return a.ServiceList()
	}
	return nil
}

func (APIConnTest) ServiceList() []*object.Service {
	svcs := []*object.Service{
		{
			Name:      "dns-service",
			Namespace: "kube-system",
			ClusterIP: "10.0.0.111",
		},
	}
	return svcs
}

func (APIConnTest) EpIndexReverse(string) []*object.Endpoints {
	eps := []*object.Endpoints{
		{
			Subsets: []object.EndpointSubset{
				{
					Addresses: []object.EndpointAddress{
						{
							IP: "127.0.0.1",
						},
					},
				},
			},
			Name:      "dns-service",
			Namespace: "kube-system",
		},
	}
	return eps
}

func (APIConnTest) GetNodeByName(name string) (*api.Node, error) { return &api.Node{}, nil }
func (APIConnTest) GetNamespaceByName(name string) (*api.Namespace, error) {
	return &api.Namespace{}, nil
}

func TestNsAddrs(t *testing.T) {

	k := New([]string{"inter.webs.test."})
	k.APIConn = &APIConnTest{}

	cdrs := k.nsAddrs(false, k.Zones[0])
	expected := "10.0.0.111"

	if len(cdrs) != 1 {
		t.Fatalf("Expected 1 result, got %q", len(cdrs))

	}
	cdr := cdrs[0]
	if cdr.(*dns.A).A.String() != expected {
		t.Errorf("Expected A to be %q, got %q", expected, cdr.(*dns.A).A.String())
	}
	expected = "dns-service.kube-system.svc.inter.webs.test."
	if cdr.Header().Name != expected {
		t.Errorf("Expected Header Name to be %q, got %q", expected, cdr.Header().Name)
	}
}
