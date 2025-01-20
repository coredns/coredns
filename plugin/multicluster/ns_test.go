package multicluster

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes"
	k8sObject "github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

func nsAddressMock(state request.Request, external, headless bool) []dns.RR {
	a := test.A(fmt.Sprintf("ns.dns.%s IN A 127.0.0.1", state.Zone))
	return []dns.RR{a}
}

func TestApex(t *testing.T) {
	k := kubernetes.New([]string{"cluster.local."})
	k.APIConn = &k8sControllerMock{}

	m := New([]string{"example.com."})
	m.controller = &controllerMock{}
	m.Next = test.NextHandler(dns.RcodeSuccess, nil)
	m.nsAddrsFunc = nsAddressMock // internal test function

	ctx := context.TODO()

	for i, tc := range testsNS {
		r := tc.Msg()

		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := m.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d, expected no error, got %v", i, err)
			return
		}
		if tc.Error != nil {
			continue
		}

		resp := w.Msg
		if resp == nil {
			t.Fatalf("Test %d, got nil message and no error ford", i)
		}

		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Errorf("Test %d: %v", i, err)
		}
	}
}

var testsNS = []test.Case{
	{
		Qname: "example.com.", Qtype: dns.TypeNS,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.NS("example.com. 5     IN      NS     ns.dns.example.com."),
		},
		Extra: []dns.RR{
			test.A("ns.dns.example.com.   5       IN      A       127.0.0.1"),
		},
	},
	{
		Qname: "example.com.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	},
	{
		Qname: "example.com.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	},
	{
		Qname: "example.com.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	},
}

type k8sControllerMock struct{}

func (k8sControllerMock) HasSynced() bool                         { return true }
func (k8sControllerMock) Run()                                    {}
func (k8sControllerMock) Stop() error                             { return nil }
func (k8sControllerMock) Modified(bool) int64                     { return 0 }
func (k8sControllerMock) EpIndex(s string) []*k8sObject.Endpoints { return nil }

func (k8sControllerMock) EpIndexReverse(string) []*k8sObject.Endpoints { return nil }
func (k8sControllerMock) SvcIndexReverse(string) []*k8sObject.Service  { return nil }

func (k8sControllerMock) EndpointsList() []*k8sObject.Endpoints { return nil }
func (k8sControllerMock) GetNodeByName(ctx context.Context, name string) (*api.Node, error) {
	return nil, nil
}
func (k8sControllerMock) SvcIndex(s string) []*k8sObject.Service { return nil }
func (k8sControllerMock) PodIndex(string) []*k8sObject.Pod       { return nil }

func (k8sControllerMock) SvcExtIndexReverse(ip string) []*k8sObject.Service { return nil }

func (k8sControllerMock) GetNamespaceByName(name string) (*k8sObject.Namespace, error) {
	return &k8sObject.Namespace{
		Name: name,
	}, nil
}

func (k8sControllerMock) ServiceList() []*k8sObject.Service { return nil }
