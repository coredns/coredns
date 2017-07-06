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

var dnsTestCases = map[string](*test.Case){
	"A Service": {
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Service (Headless)": {
		Qname: "hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.2"),
			test.A("hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.3"),
		},
	},
	"SRV Service": {
		Qname: "_http._tcp.svc1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc1.testns.svc.cluster.local.	0	IN	SRV	0 100 80 svc1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"SRV Service (Headless)": {
		Qname: "_http._tcp.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	0	IN	SRV	0 50 80 172-0-0-2.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	0	IN	SRV	0 50 80 172-0-0-3.hdls1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-0-0-2.hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.2"),
			test.A("172-0-0-3.hdls1.testns.svc.cluster.local.	0	IN	A	172.0.0.3"),
		},
	},
	// TODO A External
	"CNAME External": {
		Qname: "external.testns.svc.cluster.local.", Qtype: dns.TypeCNAME,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("external.testns.svc.cluster.local.	0	IN	CNAME	ext.interwebs.test."),
		},
	},
	"A Service (Local Federated)": {
		Qname: "svc1.testns.fed.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.testns.fed.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"PTR Service": {
		Qname: "1.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("1.0.0.10.in-addr.arpa.	0	IN	PTR	svc1.testns.svc.cluster.local."),
		},
	},
	// TODO A Service (Remote Federated)
	"CNAME Service (Remote Federated)": {
		Qname: "svc0.testns.fed.svc.cluster.local.", Qtype: dns.TypeCNAME,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc0.testns.fed.svc.cluster.local.	0	IN	CNAME	svc0.testns.fed.svc.fd-az.fd-r.federal.test."),
		},
	},
}

var autopathCases = map[string](*test.Case){
	"A Autopath Service (Second Search)": {
		Qname: "svc1.testns.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Autopath Service (Third Search)": {
		Qname: "svc1.testns.svc.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.svc.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Autopath Next Middleware (Host Domain Search)": {
		Qname: "test1.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("test1.podns.svc.cluster.local.	0	IN	CNAME	test1.hostdom.test."),
			test.A("test1.hostdom.test.	0	IN	A	11.22.33.44"),
		},
	},
	"A Autopath Service (Bare Search)": {
		Qname: "svc1.testns.svc.cluster.local.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc1.testns.svc.cluster.local.podns.svc.cluster.local.	0	IN	CNAME	svc1.testns.svc.cluster.local."),
			test.A("svc1.testns.svc.cluster.local.	0	IN	A	10.0.0.1"),
		},
	},
	"A Autopath Next Middleware (Bare Search)": {
		Qname: "test2.interwebs.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("test2.interwebs.podns.svc.cluster.local.	0	IN	CNAME	test2.interwebs."),
			test.A("test2.interwebs.	0	IN	A	55.66.77.88"),
		},
	},
}
var autopathBareSearch = map[string](*test.Case){
	"A Autopath Next Middleware (Bare Search) Non-existing OnNXDOMAIN default": {
		Qname: "nothere.interwebs.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode:  dns.RcodeSuccess,
		Answer: []dns.RR{},
	},
}
var autopathBareSearchExpectNameErr = map[string](*test.Case){
	"A Autopath Next Middleware (Bare Search) Non-existing OnNXDOMAIN disabled": {
		Qname: "nothere.interwebs.podns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode:  dns.RcodeNameError,
		Answer: []dns.RR{},
	},
}

func TestServeDNS(t *testing.T) {

	k := Kubernetes{Zones: []string{"cluster.local."}}
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")

	k.ReverseCidrs = []net.IPNet{*cidr}
	k.Federations = []Federation{{name: "fed", zone: "federal.test."}}
	k.APIConn = &APIConnServeTest{}
	k.AutoPath.Enabled = true
	k.AutoPath.HostSearchPath = []string{"hostdom.test"}
	//k.Proxy = test.MockHandler(nextMWMap)
	k.Next = test.MockHandler(nextMWMap)

	ctx := context.TODO()
	runServeDNSTests(t, dnsTestCases, k, ctx)
	runServeDNSTests(t, autopathCases, k, ctx)
	runServeDNSTests(t, autopathBareSearch, k, ctx)

	// Disable the NXDOMAIN override (enabled by default)
	k.OnNXDOMAIN = dns.RcodeNameError
	runServeDNSTests(t, autopathCases, k, ctx)
	runServeDNSTests(t, autopathBareSearchExpectNameErr, k, ctx)

}

func runServeDNSTests(t *testing.T, dnsTestCases map[string](*test.Case), k Kubernetes, ctx context.Context) {
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

		if !test.Header(t, *tc, resp) {
			t.Logf("%v Received:\n %v\n", testname, resp)
			continue
		}
		if !test.Section(t, *tc, test.Answer, resp.Answer) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
		if !test.Section(t, *tc, test.Ns, resp.Ns) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
		if !test.Section(t, *tc, test.Extra, resp.Extra) {
			t.Logf("%v Received:\n %v\n", testname, resp)
		}
	}
}

// Test data & mocks

var nextMWMap = map[dns.Question]dns.Msg{
	{
		Name: "test1.hostdom.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET,
	}: {
		Answer: []dns.RR{test.A("test1.hostdom.test.	0	IN	A	11.22.33.44")},
	},
	{
		Name: "test2.interwebs.", Qtype: dns.TypeA, Qclass: dns.ClassINET,
	}: {
		Answer: []dns.RR{test.A("test2.interwebs.	0	IN	A	55.66.77.88")},
	}}

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
