package multicluster

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	k8sObject "github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/request"
	"github.com/coredns/multicluster/object"
	"github.com/miekg/dns"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func TestEndpointHostname(t *testing.T) {
	tests := []struct {
		ip       string
		hostname string
		expected string
	}{
		{"10.11.12.13", "", "10-11-12-13"},
		{"10.11.12.13", "epname", "epname"},
	}
	for _, test := range tests {
		result := endpointHostname(k8sObject.EndpointAddress{IP: test.ip, Hostname: test.hostname})
		if result != test.expected {
			t.Errorf("Expected endpoint name for (ip:%v hostname:%v) to be '%v', but got '%v'", test.ip, test.hostname, test.expected, result)
		}
	}
}

type controllerMock struct{}

func (controllerMock) HasSynced() bool { return true }
func (controllerMock) Run()            {}
func (controllerMock) Stop() error     { return nil }
func (controllerMock) Modified() int64 { return 0 }

func (controllerMock) SvcIndex(string) []*object.ServiceImport {
	svcs := []*object.ServiceImport{
		{
			Name:       "svc1",
			Namespace:  "testns",
			ClusterIPs: []string{"10.0.0.1"},
			Ports: []mcs.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
		{
			Name:       "svc-dual-stack",
			Namespace:  "testns",
			ClusterIPs: []string{"10.0.0.2", "10::2"},
			Ports: []mcs.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
		{
			Name:      "hdls1",
			Namespace: "testns",
			Type:      mcs.Headless,
		},
	}
	return svcs
}

func (controllerMock) ServiceList() []*object.ServiceImport {
	svcs := []*object.ServiceImport{
		{
			Name:       "svc1",
			Namespace:  "testns",
			ClusterIPs: []string{"10.0.0.1"},
			Ports: []mcs.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
		{
			Name:       "svc-dual-stack",
			Namespace:  "testns",
			ClusterIPs: []string{"10.0.0.2", "10::2"},
			Ports: []mcs.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
		{
			Name:      "hdls1",
			Namespace: "testns",
			Type:      mcs.Headless,
		},
	}
	return svcs
}

func (controllerMock) EpIndex(string) []*object.Endpoints {
	eps := []*object.Endpoints{
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "172.0.0.1", Hostname: "ep1a"},
						},
						Ports: []k8sObject.EndpointPort{
							{Port: 80, Protocol: "tcp", Name: "http"},
						},
					},
				},
				Name:      "svc1-slice1",
				Namespace: "testns",
				Index:     object.EndpointsKey("svc1", "testns"),
			},
			ClusterId: "clusterid",
		},
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "172.0.0.2"},
						},
						Ports: []k8sObject.EndpointPort{
							{Port: 80, Protocol: "tcp", Name: "http"},
						},
					},
				},
				Name:      "hdls1-slice1",
				Namespace: "testns",
				Index:     object.EndpointsKey("hdls1", "testns"),
			},
			ClusterId: "clusterid",
		},
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "10.9.8.7", NodeName: "test.node.foo.bar"},
						},
					},
				},
			},
		},
	}
	return eps
}

func (controllerMock) EndpointsList() []*object.Endpoints {
	eps := []*object.Endpoints{
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "172.0.0.1", Hostname: "ep1a"},
						},
						Ports: []k8sObject.EndpointPort{
							{Port: 80, Protocol: "tcp", Name: "http"},
						},
					},
				},
				Name:      "svc1-slice1",
				Namespace: "testns",
				Index:     object.EndpointsKey("svc1", "testns"),
			},
		},
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "172.0.0.2"},
						},
						Ports: []k8sObject.EndpointPort{
							{Port: 80, Protocol: "tcp", Name: "http"},
						},
					},
				},
				Name:      "hdls1-slice1",
				Namespace: "testns",
				Index:     object.EndpointsKey("hdls1", "testns"),
			},
		},
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "172.0.0.2"},
						},
						Ports: []k8sObject.EndpointPort{
							{Port: 80, Protocol: "tcp", Name: "http"},
						},
					},
				},
				Name:      "hdls1-slice2",
				Namespace: "testns",
				Index:     object.EndpointsKey("hdls1", "testns"),
			},
		},
		{
			Endpoints: k8sObject.Endpoints{
				Subsets: []k8sObject.EndpointSubset{
					{
						Addresses: []k8sObject.EndpointAddress{
							{IP: "10.9.8.7", NodeName: "test.node.foo.bar"},
						},
					},
				},
			},
		},
	}
	return eps
}

func (controllerMock) GetNamespaceByName(name string) (*k8sObject.Namespace, error) {
	return &k8sObject.Namespace{
		Name: name,
	}, nil
}

func TestServices(t *testing.T) {
	m := New([]string{"interwebs.test."})
	m.controller = &controllerMock{}

	type svcAns struct {
		host string
		key  string
	}
	type svcTest struct {
		qname  string
		qtype  uint16
		answer []svcAns
	}
	tests := []svcTest{
		// Cluster IP Services
		{qname: "svc1.testns.svc.interwebs.test.", qtype: dns.TypeA, answer: []svcAns{{host: "10.0.0.1", key: "/" + coredns + "/test/interwebs/svc/testns/svc1"}}},
		{qname: "_http._tcp.svc1.testns.svc.interwebs.test.", qtype: dns.TypeSRV, answer: []svcAns{{host: "10.0.0.1", key: "/" + coredns + "/test/interwebs/svc/testns/svc1"}}},
		{qname: "ep1a.clusterid.svc1.testns.svc.interwebs.test.", qtype: dns.TypeA, answer: []svcAns{{host: "172.0.0.1", key: "/" + coredns + "/test/interwebs/svc/testns/svc1/clusterid/ep1a"}}},

		// Dual-Stack Cluster IP Service
		{
			qname: "_http._tcp.svc-dual-stack.testns.svc.interwebs.test.",
			qtype: dns.TypeSRV,
			answer: []svcAns{
				{host: "10.0.0.2", key: "/" + coredns + "/test/interwebs/svc/testns/svc-dual-stack"},
				{host: "10::2", key: "/" + coredns + "/test/interwebs/svc/testns/svc-dual-stack"},
			},
		},

		// Headless Services
		{qname: "hdls1.testns.svc.interwebs.test.", qtype: dns.TypeA, answer: []svcAns{{host: "172.0.0.2", key: "/" + coredns + "/test/interwebs/svc/testns/hdls1/clusterid/172-0-0-2"}}},
	}

	for i, test := range tests {
		state := request.Request{
			Req:  &dns.Msg{Question: []dns.Question{{Name: test.qname, Qtype: test.qtype}}},
			Zone: "interwebs.test.", // must match from m.Zones[0]
		}
		svcs, e := m.Services(context.TODO(), state, false, plugin.Options{})
		if e != nil {
			t.Errorf("Test %d: got error '%v'", i, e)
			continue
		}
		if len(svcs) != len(test.answer) {
			t.Errorf("Test %d, expected %v answer, got %v", i, len(test.answer), len(svcs))
			continue
		}

		for j := range svcs {
			if test.answer[j].host != svcs[j].Host {
				t.Errorf("Test %d, expected host '%v', got '%v'", i, test.answer[j].host, svcs[j].Host)
			}
			if test.answer[j].key != svcs[j].Key {
				t.Errorf("Test %d, expected key '%v', got '%v'", i, test.answer[j].key, svcs[j].Key)
			}
		}
	}
}
