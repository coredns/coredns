package kubernetes

import (
	"net"
	"sort"
	"testing"

	"github.com/coredns/coredns/middleware/pkg/dnsrecorder"
	"github.com/coredns/coredns/middleware/test"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
	"k8s.io/client-go/1.5/pkg/api"
)

var dnsTestCases = map[string](test.Case){
	"A Service": {
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Service (Headless)": {
		Qname: "hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.2"),
			test.A("hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.3"),
		},
	},
	"SRV Service": {
		Qname: "_http._tcp.svc1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc1.testns.svc.cluster.local.	0	IN	SRV	0 100 80 svc1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"SRV Service (Headless)": {
		Qname: "_http._tcp.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Answer: []dns.RR{
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	0	IN	SRV	0 50 80 172-0-0-2.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	0	IN	SRV	0 50 80 172-0-0-3.hdls1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-0-0-2.hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.2"),
			test.A("172-0-0-3.hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.3"),
		},
	},
	"CNAME External": {
		Qname: "external.testns.svc.cluster.local.", Qtype: dns.TypeCNAME,
		Answer: []dns.RR{
			test.CNAME("external.testns.svc.cluster.local.	0	IN	CNAME	ext.interwebs.test."),
		},
	},
	"A Service (Local Federated)": {
		Qname: "svc1.testns.fed.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("svc1.testns.fed.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"PTR Service": {
		Qname: "1.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Answer: []dns.RR{
			test.PTR("1.0.0.10.in-addr.arpa.	0	IN	PTR	svc1.testns.svc.cluster.local."),
		},
	},
	"CNAME Service (Remote Federated)": {
		Qname: "svc0.testns.fed.svc.cluster.local.", Qtype: dns.TypeCNAME,
		Answer: []dns.RR{
			test.CNAME("svc0.testns.fed.svc.cluster.local.	0	IN	CNAME	svc0.testns.fed.svc.fd-az.fd-r.interwebs.test."),
		},
	},
	"A Autopath Service (Second Path)": {
		Qname: "svc1.testns.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Autopath Service (Third Path)": {
		Qname: "svc1.testns.svc.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.svc.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Autopath Service ('.' Path K8s Object)": {
		Qname: "svc1.testns.svc.cluster.local.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.svc.cluster.local.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
}

func TestServeDNS(t *testing.T) {

	k := Kubernetes{Zones: []string{"cluster.local."}}
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")

	k.ReverseCidrs = []net.IPNet{*cidr}
	k.Federations = []Federation{{name: "fed", zone: "interwebs.test."}}
	k.APIConn = &APIConnServeTest{}
	k.AutoPath.Enabled = true
	k.AutoPath.HostSearchPath = []string{"outerwebs.test"}

	ctx := context.TODO()

	for testname, tc := range dnsTestCases {
		testname = "\nTest Case \"" + testname + "\""

		r := tc.Msg()

		w := dnsrecorder.New(&test.ResponseWriter{})

		_, err := k.ServeDNS(ctx, w, r)
		if err != nil {
			t.Errorf("%v expected no error, got %v\n", testname, err)
			return
		}

		resp := w.Msg
		sort.Sort(test.RRSet(resp.Answer))
		sort.Sort(test.RRSet(resp.Ns))
		sort.Sort(test.RRSet(resp.Extra))
		sort.Sort(test.RRSet(tc.Answer))
		sort.Sort(test.RRSet(tc.Ns))
		sort.Sort(test.RRSet(tc.Extra))

		if !test.Header(t, tc, resp) {
			t.Logf("%v Received:\n %v\n", testname, resp)
			continue
		}
		if !test.Section(t, tc, test.Answer, resp.Answer) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
		if !test.Section(t, tc, test.Ns, resp.Ns) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
		if !test.Section(t, tc, test.Extra, resp.Extra) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
	}
}

// Test data

const testUp = `; interwebs.test. test file for cname/upstream tests
interwebs.test.          IN      SOA     ns.interwebs.test. admin.interwebs.test. 2015082541 7200 3600 1209600 3600
svc0.testns.fed.svc.fd-az.fd-r.interwebs.test. IN CNAME svc0.testns.fed.svc.fd-r.interwebs.test.
svc0.testns.fed.svc.fd-r.interwebs.test. IN CNAME svc0.testns.fed.svc.interwebs.test.
svc0.testns.fed.svc.interwebs.test. IN A 13.14.15.16
ext.interwebs.test.	IN	A	13.14.15.16
`

type APIConnServeTest struct{}

func (APIConnServeTest) Run()        { return }
func (APIConnServeTest) Stop() error { return nil }

func (APIConnServeTest) PodIndex(string) []interface{} {
	a := make([]interface{}, 1)
	a[0] = &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Namespace: "podns",
		},
		Status: api.PodStatus{
			PodIP: "10.240.0.1", // Remote IP set in test.ResponseWriter
		},
	}
	return a
}

func (APIConnServeTest) ServiceList() []*api.Service {
	svcs := []*api.Service{
		{
			ObjectMeta: api.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
		{
			ObjectMeta: api.ObjectMeta{
				Name:      "hdls1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: api.ClusterIPNone,
			},
		},
		{
			ObjectMeta: api.ObjectMeta{
				Name:      "external",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ExternalName: "ext.interwebs.test",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
	}
	return svcs

}

func (APIConnServeTest) EndpointsList() api.EndpointsList {
	n := "test.node.foo.bar"

	return api.EndpointsList{
		Items: []api.Endpoints{
			{
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP:       "172.0.0.1",
								Hostname: "ep1a",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     80,
								Protocol: "tcp",
								Name:     "http",
							},
						},
					},
				},
				ObjectMeta: api.ObjectMeta{
					Name:      "svc1",
					Namespace: "testns",
				},
			},
			{
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: "172.0.0.2",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     80,
								Protocol: "tcp",
								Name:     "http",
							},
						},
					},
				},
				ObjectMeta: api.ObjectMeta{
					Name:      "hdls1",
					Namespace: "testns",
				},
			},
			{
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: "172.0.0.3",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     80,
								Protocol: "tcp",
								Name:     "http",
							},
						},
					},
				},
				ObjectMeta: api.ObjectMeta{
					Name:      "hdls1",
					Namespace: "testns",
				},
			},
			{
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP:       "10.9.8.7",
								NodeName: &n,
							},
						},
					},
				},
			},
		},
	}
}

func (APIConnServeTest) GetNodeByName(name string) (api.Node, error) {
	return api.Node{
		ObjectMeta: api.ObjectMeta{
			Name: "test.node.foo.bar",
			Labels: map[string]string{
				labelRegion:           "fd-r",
				labelAvailabilityZone: "fd-az",
			},
		},
	}, nil
}
