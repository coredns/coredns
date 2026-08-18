package kubernetes

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type APIConnReverseTest struct{}

func (APIConnReverseTest) HasSynced() bool                                  { return true }
func (APIConnReverseTest) Run()                                             {}
func (APIConnReverseTest) Stop() error                                      { return nil }
func (APIConnReverseTest) PodIndex(string) []*object.Pod                    { return nil }
func (APIConnReverseTest) EpIndex(string) []*object.Endpoints               { return nil }
func (APIConnReverseTest) McEpIndex(string) []*object.MultiClusterEndpoints { return nil }
func (APIConnReverseTest) EndpointsList() []*object.Endpoints               { return nil }
func (APIConnReverseTest) ServiceList() []*object.Service                   { return nil }
func (APIConnReverseTest) ServiceImportList() []*object.ServiceImport       { return nil }
func (APIConnReverseTest) SvcImportIndex(string) []*object.ServiceImport    { return nil }
func (APIConnReverseTest) SvcExtIndexReverse(string) []*object.Service      { return nil }
func (APIConnReverseTest) Modified(ModifiedMode) int64                      { return 0 }

func (APIConnReverseTest) SvcIndex(svc string) []*object.Service {
	if svc != "svc1.testns" {
		return nil
	}
	svcs := []*object.Service{
		{
			Name:       "svc1",
			Namespace:  "testns",
			ClusterIPs: []string{"192.168.1.100"},
			Ports:      []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	}
	return svcs
}

func (APIConnReverseTest) SvcIndexReverse(ip string) []*object.Service {
	if ip != "192.168.1.100" {
		return nil
	}
	svcs := []*object.Service{
		{
			Name:       "svc1",
			Namespace:  "testns",
			ClusterIPs: []string{"192.168.1.100"},
			Ports:      []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	}
	return svcs
}

func (APIConnReverseTest) EpIndexReverse(ip string) []*object.Endpoints {
	ep1s1 := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "10.0.0.100", Hostname: "ep1a"},
					{IP: "10.0.0.99", Hostname: "double-ep"}, // this endpoint is used by two services
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		Name:      "svc1-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svc1", "testns"),
	}
	ep1s2 := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "1234:abcd::1", Hostname: "ep1b"},
					{IP: "fd00:77:30::a", Hostname: "ip6svc1ex"},
					{IP: "fd00:77:30::2:9ba6", Hostname: "ip6svc1in"},
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		Name:      "svc1-slice2",
		Namespace: "testns",
		Index:     object.EndpointsKey("svc1", "testns"),
	}
	ep1s3 := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "10.0.0.100", Hostname: "ep1a"}, // duplicate endpointslice address
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		Name:      "svc1-ccccc",
		Namespace: "testns",
		Index:     object.EndpointsKey("svc1", "testns"),
	}
	ep2 := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "10.0.0.99", Hostname: "double-ep"}, // this endpoint is used by two services
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		Name:      "svc2-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svc2", "testns"),
	}
	switch ip {
	case "1234:abcd::1":
		fallthrough
	case "fd00:77:30::a":
		fallthrough
	case "fd00:77:30::2:9ba6":
		return []*object.Endpoints{&ep1s2}
	case "10.0.0.100": // two EndpointSlices for a Service contain this IP (EndpointSlice skew)
		return []*object.Endpoints{&ep1s1, &ep1s3}
	case "10.0.0.99": // two different Services select this IP
		return []*object.Endpoints{&ep1s1, &ep2}
	}
	return nil
}

// APIConnReversePTRHostnameOnlyTest models a pod IP selected by two services, where only one of
// the endpoints has a hostname set (e.g. a StatefulSet pod behind its headless service plus an
// individual LoadBalancer service). It reuses APIConnReverseTest for everything but the reverse
// endpoint lookup.
type APIConnReversePTRHostnameOnlyTest struct{ APIConnReverseTest }

func (APIConnReversePTRHostnameOnlyTest) EpIndexReverse(ip string) []*object.Endpoints {
	if ip != "10.0.0.50" {
		return nil
	}
	withHostname := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{{IP: "10.0.0.50", Hostname: "pod-0"}},
				Ports:     []object.EndpointPort{{Port: 80, Protocol: "tcp", Name: "http"}},
			},
		},
		Name:      "svca-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svca", "testns"),
	}
	withoutHostname := object.Endpoints{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{{IP: "10.0.0.50", Hostname: ""}},
				Ports:     []object.EndpointPort{{Port: 80, Protocol: "tcp", Name: "http"}},
			},
		},
		Name:      "svcb-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svcb", "testns"),
	}
	return []*object.Endpoints{&withHostname, &withoutHostname}
}

func TestReversePTRHostnameOnly(t *testing.T) {
	zones := []string{"cluster.local.", "0.10.in-addr.arpa."}

	// Default: every endpoint gets a PTR, using the IP-derived name when no hostname is set.
	defaultCase := test.Case{
		Qname: "50.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("50.0.0.10.in-addr.arpa.      5    IN      PTR       10-0-0-50.svcb.testns.svc.cluster.local."),
			test.PTR("50.0.0.10.in-addr.arpa.      5    IN      PTR       pod-0.svca.testns.svc.cluster.local."),
		},
	}

	// ptr_hostname_only: only the endpoint with a hostname gets a PTR; the IP-derived one is dropped.
	hostnameOnlyCase := test.Case{
		Qname: "50.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("50.0.0.10.in-addr.arpa.      5    IN      PTR       pod-0.svca.testns.svc.cluster.local."),
		},
	}

	for _, tt := range []struct {
		name            string
		ptrHostnameOnly bool
		tc              test.Case
	}{
		{"default", false, defaultCase},
		{"ptr_hostname_only", true, hostnameOnlyCase},
	} {
		t.Run(tt.name, func(t *testing.T) {
			k := New(zones)
			k.APIConn = &APIConnReversePTRHostnameOnlyTest{}
			k.ptrHostnameOnly = tt.ptrHostnameOnly

			w := dnstest.NewRecorder(&test.ResponseWriter{})
			if _, err := k.ServeDNS(context.TODO(), w, tt.tc.Msg()); err != tt.tc.Error {
				t.Fatalf("expected error %v, got %v", tt.tc.Error, err)
			}
			if w.Msg == nil {
				t.Fatal("got nil message")
			}
			if err := test.SortAndCheck(w.Msg, tt.tc); err != nil {
				t.Error(err)
			}
		})
	}
}

func (APIConnReverseTest) GetNodeByName(_ctx context.Context, _name string) (*api.Node, error) {
	return &api.Node{
		ObjectMeta: meta.ObjectMeta{
			Name: "test.node.foo.bar",
		},
	}, nil
}

func (APIConnReverseTest) GetNamespaceByName(name string) (*object.Namespace, error) {
	return &object.Namespace{
		Name: name,
	}, nil
}

func TestReverse(t *testing.T) {
	k := New([]string{"cluster.local.", "0.10.in-addr.arpa.", "168.192.in-addr.arpa.", "0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.d.c.b.a.4.3.2.1.ip6.arpa.", "0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa."})
	k.APIConn = &APIConnReverseTest{}

	tests := []test.Case{
		{
			Qname: "100.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("100.0.0.10.in-addr.arpa.      5    IN      PTR       ep1a.svc1.testns.svc.cluster.local."),
			},
		},
		{
			Qname: "100.1.168.192.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("100.1.168.192.in-addr.arpa.     5     IN      PTR       svc1.testns.svc.cluster.local."),
			},
		},
		{ // A PTR record query for an existing ipv6 endpoint should return a record
			Qname: "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.d.c.b.a.4.3.2.1.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.d.c.b.a.4.3.2.1.ip6.arpa. 5 IN PTR ep1b.svc1.testns.svc.cluster.local."),
			},
		},
		{ // A PTR record query for an existing ipv6 endpoint should return a record
			Qname: "a.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("a.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa. 5 IN PTR ip6svc1ex.svc1.testns.svc.cluster.local."),
			},
		},
		{ // A PTR record query for an existing ipv6 endpoint should return a record
			Qname: "6.a.b.9.2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("6.a.b.9.2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa. 5 IN PTR ip6svc1in.svc1.testns.svc.cluster.local."),
			},
		},
		{
			Qname: "101.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeNameError,
			Ns: []dns.RR{
				test.SOA("0.10.in-addr.arpa.	5	IN	SOA	ns.dns.0.10.in-addr.arpa. hostmaster.0.10.in-addr.arpa. 1502782828 7200 1800 86400 5"),
			},
		},
		{
			Qname: "example.org.cluster.local.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeNameError,
			Ns: []dns.RR{
				test.SOA("cluster.local.       5     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1502989566 7200 1800 86400 5"),
			},
		},
		{
			Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Ns: []dns.RR{
				test.SOA("cluster.local.       5     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1502989566 7200 1800 86400 5"),
			},
		},
		{
			Qname: "svc1.testns.svc.0.10.in-addr.arpa.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns: []dns.RR{
				test.SOA("0.10.in-addr.arpa.       5     IN      SOA     ns.dns.0.10.in-addr.arpa. hostmaster.0.10.in-addr.arpa. 1502989566 7200 1800 86400 5"),
			},
		},
		{
			Qname: "100.0.0.10.cluster.local.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeNameError,
			Ns: []dns.RR{
				test.SOA("cluster.local.       5     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1502989566 7200 1800 86400 5"),
			},
		},
		{
			Qname: "99.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("99.0.0.10.in-addr.arpa.      5    IN      PTR       double-ep.svc1.testns.svc.cluster.local."),
				test.PTR("99.0.0.10.in-addr.arpa.      5    IN      PTR       double-ep.svc2.testns.svc.cluster.local."),
			},
		},
	}

	ctx := context.TODO()
	for i, tc := range tests {
		r := tc.Msg()

		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := k.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d: expected no error, got %v", i, err)
			return
		}

		resp := w.Msg
		if resp == nil {
			t.Fatalf("Test %d: got nil message and no error for: %s %d", i, r.Question[0].Name, r.Question[0].Qtype)
		}
		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Error(err)
		}
	}
}
