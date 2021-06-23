package external

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

func TestExternal(t *testing.T) {
	k := kubernetes.New([]string{"cluster.local."})
	k.Namespaces = map[string]struct{}{"testns": {}}
	k.APIConn = &external{}

	e := New()
	e.Zones = []string{"example.com."}
	e.Next = test.NextHandler(dns.RcodeSuccess, nil)
	e.externalFunc = k.External
	e.externalAddrFunc = externalAddress  // internal test function
	e.externalSerialFunc = externalSerial // internal test function

	ctx := context.TODO()
	for i, tc := range tests {
		r := tc.Msg()
		w := dnstest.NewRecorder(&test.ResponseWriter{})

		e.upstream = tc.Upstream

		_, err := e.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d expected no error, got %v", i, err)
			return
		}
		if tc.Error != nil {
			continue
		}

		resp := w.Msg

		if resp == nil {
			t.Fatalf("Test %d, got nil message and no error for %q", i, r.Question[0].Name)
		}

		if resp.Truncated != tc.Truncated {
			t.Errorf("Test %d expected TC=%v, got TC=%v", i, tc.Truncated, resp.Truncated)
		}

		if err = test.SortAndCheck(resp, tc.Case); err != nil {
			t.Error(err)
		}
	}
}

type kubeTestCase struct {
	Upstream  Upstreamer
	Truncated bool
	test.Case
}

var tests = []kubeTestCase{
	// A Service
	{Case: test.Case{
		Qname: "svc1.testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.testns.example.com.	5	IN	A	1.2.3.4"),
		},
	}},
	{Case: test.Case{
		Qname: "svc1.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svc1.testns.example.com.	5	IN	SRV	0 100 80 svc1.testns.example.com.")},
		Extra:  []dns.RR{test.A("svc1.testns.example.com.  5       IN      A       1.2.3.4")},
	}},
	// SRV Service Not udp/tcp
	{Case: test.Case{
		Qname: "*._not-udp-or-tcp.svc1.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// SRV Service
	{Case: test.Case{
		Qname: "_http._tcp.svc1.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc1.testns.example.com.	5	IN	SRV	0 100 80 svc1.testns.example.com."),
		},
		Extra: []dns.RR{
			test.A("svc1.testns.example.com.	5	IN	A	1.2.3.4"),
		},
	}},
	// AAAA Service (with an existing A record, but no AAAA record)
	{Case: test.Case{
		Qname: "svc1.testns.example.com.", Qtype: dns.TypeAAAA, Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// AAAA Service (non-existing service)
	{Case: test.Case{
		Qname: "svc0.testns.example.com.", Qtype: dns.TypeAAAA, Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// A Service (non-existing service)
	{Case: test.Case{
		Qname: "svc0.testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// A Service (non-existing namespace)
	{Case: test.Case{
		Qname: "svc0.svc-nons.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// AAAA Service
	{Case: test.Case{
		Qname: "svc6.testns.example.com.", Qtype: dns.TypeAAAA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("svc6.testns.example.com.	5	IN	AAAA	1:2::5"),
		},
	}},
	// SRV
	{Case: test.Case{
		Qname: "_http._tcp.svc6.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc6.testns.example.com.	5	IN	SRV	0 100 80 svc6.testns.example.com."),
		},
		Extra: []dns.RR{
			test.AAAA("svc6.testns.example.com.	5	IN	AAAA	1:2::5"),
		},
	}},
	// SRV
	{Case: test.Case{
		Qname: "svc6.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("svc6.testns.example.com.	5	IN	SRV	0 100 80 svc6.testns.example.com."),
		},
		Extra: []dns.RR{
			test.AAAA("svc6.testns.example.com.	5	IN	AAAA	1:2::5"),
		},
	}},
	{Case: test.Case{
		Qname: "testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	{Case: test.Case{
		Qname: "testns.example.com.", Qtype: dns.TypeSOA, Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("example.com.	5	IN	SOA	ns1.dns.example.com. hostmaster.example.com. 1499347823 7200 1800 86400 5"),
		},
	}},
	// svc11
	{Case: test.Case{
		Qname: "svc11.testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc11.testns.example.com.	5	IN	A	1.2.3.4"),
		},
	}},
	{Case: test.Case{
		Qname: "_http._tcp.svc11.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc11.testns.example.com.	5	IN	SRV	0 100 80 svc11.testns.example.com."),
		},
		Extra: []dns.RR{
			test.A("svc11.testns.example.com.	5	IN	A	1.2.3.4"),
		},
	}},
	{Case: test.Case{
		Qname: "svc11.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("svc11.testns.example.com.	5	IN	SRV	0 100 80 svc11.testns.example.com."),
		},
		Extra: []dns.RR{
			test.A("svc11.testns.example.com.	5	IN	A	1.2.3.4"),
		},
	}},

	// svc12
	{Case: test.Case{
		Qname: "svc12.testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("dummy.hostname.	300	IN	A	1.2.3.4"),
			test.CNAME("svc12.testns.example.com.	5	IN	CNAME	dummy.hostname"),
		},
	},
		Truncated: false,
		Upstream:  &UpTrunc{Trunc: false},
	},
	{Case: test.Case{
		Qname: "_http._tcp.svc12.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
	}},
	{Case: test.Case{
		Qname: "svc12.testns.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("dummy.hostname.	300	IN	A	1.2.3.4"),
			test.CNAME("svc12.testns.example.com.	5	IN	CNAME	dummy.hostname"),
		},
	},
		Truncated: true,
		Upstream:  &UpTrunc{Trunc: true},
	},
	{Case: test.Case{
		Qname: "_http._tcp.svc12.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc12.testns.example.com.	5	IN	SRV	0 100 80 dummy.hostname."),
		},
	}},
	{Case: test.Case{
		Qname: "svc12.testns.example.com.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("svc12.testns.example.com.	5	IN	SRV	0 100 80 dummy.hostname."),
		},
	}},
}

type external struct{}

func (external) HasSynced() bool                                                   { return true }
func (external) Run()                                                              {}
func (external) Stop() error                                                       { return nil }
func (external) EpIndexReverse(string) []*object.Endpoints                         { return nil }
func (external) SvcIndexReverse(string) []*object.Service                          { return nil }
func (external) Modified(bool) int64                                               { return 0 }
func (external) EpIndex(s string) []*object.Endpoints                              { return nil }
func (external) EndpointsList() []*object.Endpoints                                { return nil }
func (external) GetNodeByName(ctx context.Context, name string) (*api.Node, error) { return nil, nil }
func (external) SvcIndex(s string) []*object.Service                               { return svcIndexExternal[s] }
func (external) PodIndex(string) []*object.Pod                                     { return nil }

func (external) GetNamespaceByName(name string) (*object.Namespace, error) {
	return &object.Namespace{
		Name: name,
	}, nil
}

var svcIndexExternal = map[string][]*object.Service{
	"svc1.testns": {
		{
			Name:        "svc1",
			Namespace:   "testns",
			Type:        api.ServiceTypeClusterIP,
			ClusterIPs:  []string{"10.0.0.1"},
			ExternalIPs: []string{"1.2.3.4"},
			Ports:       []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	},
	"svc6.testns": {
		{
			Name:        "svc6",
			Namespace:   "testns",
			Type:        api.ServiceTypeClusterIP,
			ClusterIPs:  []string{"10.0.0.3"},
			ExternalIPs: []string{"1:2::5"},
			Ports:       []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	},
	"svc11.testns": {
		{
			Name:        "svc11",
			Namespace:   "testns",
			Type:        api.ServiceTypeLoadBalancer,
			ExternalIPs: []string{"1.2.3.4"},
			Ports:       []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	},
	"svc12.testns": {
		{
			Name:        "svc12",
			Namespace:   "testns",
			Type:        api.ServiceTypeLoadBalancer,
			ExternalIPs: []string{"dummy.hostname"},
			Ports:       []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	},
}

func (external) ServiceList() []*object.Service {
	var svcs []*object.Service
	for _, svc := range svcIndexExternal {
		svcs = append(svcs, svc...)
	}
	return svcs
}

func externalAddress(state request.Request) []dns.RR {
	a := test.A("example.org. IN A 127.0.0.1")
	return []dns.RR{a}
}

func externalSerial(string) uint32 {
	return 1499347823
}

// UpTrunc implements an Upstreamer that controls truncation bit for test purposes
type UpTrunc struct {
	Trunc bool
}

// Lookup returns a hard-coded response with TC bit set to Trunc
func (t *UpTrunc) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response:  true,
			Truncated: t.Trunc,
			Rcode:     dns.RcodeSuccess,
		},
		Question: []dns.Question{{Name: "dummy.hostname.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
		Answer:   []dns.RR{test.A("dummy.hostname.	300	IN	A	1.2.3.4")},
	}, nil
}
