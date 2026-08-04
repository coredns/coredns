package kubernetes

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

type APIConnZonalTest struct{}

func (APIConnZonalTest) HasSynced() bool                                  { return true }
func (APIConnZonalTest) Run()                                             {}
func (APIConnZonalTest) Stop() error                                      { return nil }
func (APIConnZonalTest) PodIndex(string) []*object.Pod                    { return nil }
func (APIConnZonalTest) SvcIndexReverse(string) []*object.Service         { return nil }
func (APIConnZonalTest) SvcExtIndexReverse(string) []*object.Service      { return nil }
func (APIConnZonalTest) ServiceImportList() []*object.ServiceImport       { return nil }
func (APIConnZonalTest) SvcImportIndex(string) []*object.ServiceImport    { return nil }
func (APIConnZonalTest) EpIndexReverse(string) []*object.Endpoints        { return nil }
func (APIConnZonalTest) McEpIndex(string) []*object.MultiClusterEndpoints { return nil }
func (APIConnZonalTest) Modified(ModifiedMode) int64                      { return int64(1499347823) }

func (a APIConnZonalTest) ServiceList() []*object.Service {
	return []*object.Service{
		{
			Name:       "hdls",
			Namespace:  "testns",
			ClusterIPs: []string{api.ClusterIPNone},
		},
		{
			Name:       "clstr",
			Namespace:  "testns",
			ClusterIPs: []string{"10.0.0.10"},
			Ports:      []api.ServicePort{{Name: "http", Protocol: "tcp", Port: 80}},
		},
	}
}

func (a APIConnZonalTest) SvcIndex(idx string) []*object.Service {
	switch idx {
	case "hdls.testns":
		return a.ServiceList()[:1]
	case "clstr.testns":
		return a.ServiceList()[1:]
	}
	return nil
}

func (a APIConnZonalTest) EndpointsList() []*object.Endpoints {
	return []*object.Endpoints{
		{
			Subsets: []object.EndpointSubset{
				{
					Addresses: []object.EndpointAddress{
						{IP: "172.0.0.1", Zone: "us-west-2a"},
						{IP: "172.0.0.2", Zone: "us-west-2b"},
						{IP: "172.0.0.3", Zone: "us-west-2b"},
					},
					Ports: []object.EndpointPort{{Port: 80, Name: "http", Protocol: "tcp"}},
				},
			},
			Name:      "hdls-slice",
			Namespace: "testns",
			Index:     object.EndpointsKey("hdls", "testns"),
		},
	}
}

func (a APIConnZonalTest) EpIndex(idx string) []*object.Endpoints {
	if idx == "hdls.testns" {
		return a.EndpointsList()
	}
	return nil
}

func (APIConnZonalTest) GetNodeByName(ctx context.Context, name string) (*api.Node, error) {
	return &api.Node{}, nil
}

func (APIConnZonalTest) GetNamespaceByName(name string) (*object.Namespace, error) {
	return &object.Namespace{Name: name}, nil
}

var zonalTestCases = []test.Case{
	{ // pin: endpoints narrowed to the requested zone
		Qname: "us-west-2a.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("us-west-2a.pin._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.1"),
		},
	},
	{
		Qname: "us-west-2b.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("us-west-2b.pin._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("us-west-2b.pin._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
		},
	},
	{ // pin: any zone label without matching endpoints — drained zones and
		// typos alike — is the same determinate empty answer
		Qname: "us-west-2c.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // prefer: narrows exactly like pin when the zone is populated
		Qname: "us-west-2a.prefer._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("us-west-2a.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.1"),
		},
	},
	{ // prefer: an empty zone falls back to every endpoint — the fallback
		// is chosen in the name, so it is not a silent widening of a pin
		Qname: "us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.1"),
			test.A("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
		},
	},
	{ // a nonexistent service stays NXDOMAIN under any directive
		Qname: "us-west-2a.pin._zone.ghost.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // an unknown directive keeps the stock too-long NXDOMAIN
		Qname: "us-west-2a.florp._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // zone-scoped names are defined for headless services only
		Qname: "us-west-2a.pin._zone.clstr.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // pin: exists-but-empty stays NODATA for non-address qtypes too (TXT
		// has its own lookup branch; NXDOMAIN would be negative-cached per name)
		Qname: "us-west-2c.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // prefer: SRV narrows when populated and falls back when not, same as A
		Qname: "us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	SRV	0 33 80 172-0-0-1.hdls.testns.svc.cluster.local."),
			test.SRV("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	SRV	0 33 80 172-0-0-2.hdls.testns.svc.cluster.local."),
			test.SRV("us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.	5	IN	SRV	0 33 80 172-0-0-3.hdls.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-0-0-1.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.1"),
			test.A("172-0-0-2.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("172-0-0-3.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
		},
	},
	{ // prefer: TXT on a zonal name is NODATA like every non-address type
		Qname: "us-west-2c.prefer._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{ // pin: SRV comes out zone-filtered, since filtering happens at endpoint selection
		Qname: "us-west-2b.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("us-west-2b.pin._zone.hdls.testns.svc.cluster.local.	5	IN	SRV	0 50 80 172-0-0-2.hdls.testns.svc.cluster.local."),
			test.SRV("us-west-2b.pin._zone.hdls.testns.svc.cluster.local.	5	IN	SRV	0 50 80 172-0-0-3.hdls.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-0-0-2.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("172-0-0-3.hdls.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
		},
	},
}

// Without the option the shape is three-labels-too-long, exactly as it has
// always been: behavior identical to before the feature existed.
var zonalDisabledTestCases = []test.Case{
	{
		Qname: "us-west-2a.pin._zone.hdls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
}

func TestServeDNSZonal(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnZonalTest{}
	k.Next = test.NextHandler(dns.RcodeSuccess, nil)
	k.Namespaces = map[string]struct{}{"testns": {}}
	k.opts.zonal = true
	ctx := context.TODO()

	runZonalCases(ctx, t, k, zonalTestCases)

	k.opts.zonal = false
	runZonalCases(ctx, t, k, zonalDisabledTestCases)
}

func runZonalCases(ctx context.Context, t *testing.T, k *Kubernetes, cases []test.Case) {
	t.Helper()
	for i, tc := range cases {
		r := tc.Msg()
		w := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := k.ServeDNS(ctx, w, r); err != nil {
			t.Errorf("Test %d expected no error, got %v", i, err)
			continue
		}
		if w.Msg == nil {
			t.Fatalf("Test %d, got nil message for %q", i, r.Question[0].Name)
		}
		if err := test.SortAndCheck(w.Msg, tc); err != nil {
			t.Errorf("Test %d (%s), %v", i, tc.Qname, err)
		}
	}
}
