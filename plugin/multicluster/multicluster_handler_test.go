package multicluster

import (
	"context"
	"fmt"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/coredns/coredns/plugin/multicluster/object"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"testing"
)

var dnsTestCases = []test.Case{
	// A Service
	{
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	5	IN	A	10.0.0.1"),
		},
	},
	{
		Qname: "svcempty.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svcempty.testns.svc.cluster.local.	5	IN	A	10.0.0.1"),
		},
	},
	// A Service (wildcard)
	{
		Qname: "svc1.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.*.svc.cluster.local.  5       IN      A       10.0.0.1"),
		},
	},
	{
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svc1.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svc1.testns.svc.cluster.local.")},
		Extra: []dns.RR{test.A("svc1.testns.svc.cluster.local.  5       IN      A       10.0.0.1")},
	},
	{
		Qname: "svcempty.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svcempty.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svcempty.testns.svc.cluster.local.")},
		Extra: []dns.RR{test.A("svcempty.testns.svc.cluster.local.  5       IN      A       10.0.0.1")},
	},
	{
		Qname: "svc6.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svc6.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svc6.testns.svc.cluster.local.")},
		Extra: []dns.RR{test.AAAA("svc6.testns.svc.cluster.local.  5       IN      AAAA       1234:abcd::1")},
	},
	// SRV Service (wildcard)
	{
		Qname: "svc1.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svc1.*.svc.cluster.local.	5	IN	SRV	0 100 80 svc1.testns.svc.cluster.local.")},
		Extra: []dns.RR{test.A("svc1.testns.svc.cluster.local.  5       IN      A       10.0.0.1")},
	},
	{
		Qname: "svcempty.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svcempty.*.svc.cluster.local.	5	IN	SRV	0 100 80 svcempty.testns.svc.cluster.local.")},
		Extra: []dns.RR{test.A("svcempty.testns.svc.cluster.local.  5       IN      A       10.0.0.1")},
	},
	// SRV Service Not udp/tcp
	{
		Qname: "*._not-udp-or-tcp.svc1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// SRV Service
	{
		Qname: "_http._tcp.svc1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc1.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svc1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	5	IN	A	10.0.0.1"),
		},
	},
	{
		Qname: "_http._tcp.svcempty.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svcempty.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svcempty.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svcempty.testns.svc.cluster.local.	5	IN	A	10.0.0.1"),
		},
	},
	// A Service (Headless)
	{
		Qname: "hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.4"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.5"),
		},
	},
	// A Service (Headless and Portless)
	{
		Qname: "hdlsprtls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("hdlsprtls.testns.svc.cluster.local.	5	IN	A	172.0.0.20"),
		},
	},
	// An Endpoint with no port
	{
		Qname: "172-0-0-20.clusterid.hdlsprtls.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("172-0-0-20.clusterid.hdlsprtls.testns.svc.cluster.local.	5	IN	A	172.0.0.20"),
		},
	},
	// An Endpoint ip
	{
		Qname: "172-0-0-2.clusterid.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("172-0-0-2.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
		},
	},
	// A Endpoint ip
	{
		Qname: "172-0-0-3.clusterid.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("172-0-0-3.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
		},
	},
	// An Endpoint by name
	{
		Qname: "dup-name.clusterid.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("dup-name.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.4"),
			test.A("dup-name.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.5"),
		},
	},
	// SRV Service (Headless)
	{
		Qname: "_http._tcp.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	5	IN	SRV	0 16 80 172-0-0-2.clusterid.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	5	IN	SRV	0 16 80 172-0-0-3.clusterid.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	5	IN	SRV	0 16 80 5678-abcd--1.clusterid.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	5	IN	SRV	0 16 80 5678-abcd--2.clusterid.hdls1.testns.svc.cluster.local."),
			test.SRV("_http._tcp.hdls1.testns.svc.cluster.local.	5	IN	SRV	0 16 80 dup-name.clusterid.hdls1.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-0-0-2.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("172-0-0-3.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
			test.AAAA("5678-abcd--1.clusterid.hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::1"),
			test.AAAA("5678-abcd--2.clusterid.hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::2"),
			test.A("dup-name.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.4"),
			test.A("dup-name.clusterid.hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.5"),
		},
	},
	{ // An A record query for an existing headless service should return a record for each of its ipv4 endpoints
		Qname: "hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.2"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.3"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.4"),
			test.A("hdls1.testns.svc.cluster.local.	5	IN	A	172.0.0.5"),
		},
	},
	// SRV Service (Headless and portless)
	{
		Qname: "*.*.hdlsprtls.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// AAAA
	{
		Qname: "5678-abcd--2.clusterid.hdls1.testns.svc.cluster.local", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.AAAA("5678-abcd--2.clusterid.hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::2")},
	},
	// AAAA Service (with an existing A record, but no AAAA record)
	{
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// AAAA Service (non-existing service)
	{
		Qname: "svc0.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// A Service (non-existing service)
	{
		Qname: "svc0.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// A Service (non-existing namespace)
	{
		Qname: "svc0.svc-nons.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// TXT Schema
	{
		Qname: "dns-version.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.TXT("dns-version.cluster.local 28800 IN TXT 1.1.0"),
		},
	},
	// A Service (Headless) does not exist
	{
		Qname: "bogusendpoint.clusterid.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// A Service does not exist
	{
		Qname: "bogusendpoint.clusterid.svc0.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// AAAA Service
	{
		Qname: "svc6.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("svc6.testns.svc.cluster.local.	5	IN	AAAA	1234:abcd::1"),
		},
	},
	// SRV
	{
		Qname: "_http._tcp.svc6.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.svc6.testns.svc.cluster.local.	5	IN	SRV	0 100 80 svc6.testns.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.AAAA("svc6.testns.svc.cluster.local.	5	IN	AAAA	1234:abcd::1"),
		},
	},
	// AAAA Service (Headless)
	{
		Qname: "hdls1.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::1"),
			test.AAAA("hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::2"),
		},
	},
	// AAAA Endpoint
	{
		Qname: "5678-abcd--1.clusterid.hdls1.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("5678-abcd--1.clusterid.hdls1.testns.svc.cluster.local.	5	IN	AAAA	5678:abcd::1"),
		},
	},

	{
		Qname: "svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{
		Qname: "pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	{
		Qname: "testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
	// Dual Stack ClusterIP Services
	{
		Qname: "svc-dual-stack.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-dual-stack.testns.svc.cluster.local.	5	IN	A	10.0.0.3"),
		},
	},
	{
		Qname: "svc-dual-stack.testns.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("svc-dual-stack.testns.svc.cluster.local.	5	IN	AAAA	10::3"),
		},
	},
	{
		Qname: "svc-dual-stack.testns.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{test.SRV("svc-dual-stack.testns.svc.cluster.local.	5	IN	SRV	0 50 80 svc-dual-stack.testns.svc.cluster.local.")},
		Extra: []dns.RR{
			test.A("svc-dual-stack.testns.svc.cluster.local.  5       IN      A       10.0.0.3"),
			test.AAAA("svc-dual-stack.testns.svc.cluster.local.  5       IN      AAAA       10::3"),
		},
	},
	{
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeSOA,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
}

func TestServeDNS(t *testing.T) {

	m := New([]string{"cluster.local."})
	m.controller = &controllerMock2{}
	m.Next = test.NextHandler(dns.RcodeSuccess, nil)
	ctx := context.TODO()

	for i, tc := range dnsTestCases {
		r := tc.Msg()

		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := m.ServeDNS(ctx, w, r)
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

		// Before sorting, make sure that CNAMES do not appear after their target records
		if err := test.CNAMEOrder(resp); err != nil {
			t.Error(err)
		}

		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Error(err, tc)
		}
	}
}

var nsTestCases = []test.Case{
	// A Service for an "exposed" namespace that "does exist"
	{
		Qname: "svc1.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc1.testns.svc.cluster.local.	5	IN	A	10.0.0.1"),
		},
	},
	// A service for an "exposed" namespace that "doesn't exist"
	{
		Qname: "svc1.nsnoexist.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	300	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1551484803 7200 1800 86400 30"),
		},
	},
}

func TestServeNamespaceDNS(t *testing.T) {
	m := New([]string{"cluster.local."})
	m.controller = &controllerMock{}
	m.Next = test.NextHandler(dns.RcodeSuccess, nil)
	ctx := context.TODO()

	for i, tc := range nsTestCases {
		r := tc.Msg()

		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := m.ServeDNS(ctx, w, r)
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

		// Before sorting, make sure that CNAMES do not appear after their target records
		test.CNAMEOrder(resp)

		test.SortAndCheck(resp, tc)
	}
}

var notSyncedTestCases = []test.Case{
	{
		// We should get ServerFailure instead of NameError for missing records when we kubernetes hasn't synced
		Qname: "svc0.testns.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeServerFailure,
		Ns: []dns.RR{
			test.SOA("cluster.local.	5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 5"),
		},
	},
}

func TestNotSyncedServeDNS(t *testing.T) {

	m := New([]string{"cluster.local."})
	m.controller = &controllerMock2{
		notSynced: true,
	}
	m.Next = test.NextHandler(dns.RcodeSuccess, nil)
	ctx := context.TODO()

	for i, tc := range notSyncedTestCases {
		r := tc.Msg()

		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := m.ServeDNS(ctx, w, r)
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

		if err := test.CNAMEOrder(resp); err != nil {
			t.Error(err)
		}

		if err := test.SortAndCheck(resp, tc); err != nil {
			t.Error(err)
		}
	}
}

type controllerMock2 struct {
	notSynced bool
}

func (a controllerMock2) HasSynced() bool { return !a.notSynced }
func (controllerMock2) Run()              {}
func (controllerMock2) Stop() error       { return nil }
func (controllerMock2) Modified() int64   { return int64(3) }

var svcIndex = map[string][]*object.ServiceImport{
	"kubedns.kube-system": {
		{
			Name:       "kubedns",
			Namespace:  "kube-system",
			Type:       v1alpha1.ClusterSetIP,
			ClusterIPs: []string{"10.0.0.10"},
			Ports: []v1alpha1.ServicePort{
				{Name: "dns", Protocol: "udp", Port: 53},
			},
		},
	},
	"svc1.testns": {
		{
			Name:       "svc1",
			Namespace:  "testns",
			Type:       v1alpha1.ClusterSetIP,
			ClusterIPs: []string{"10.0.0.1"},
			Ports: []v1alpha1.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
	},
	"svcempty.testns": {
		{
			Name:       "svcempty",
			Namespace:  "testns",
			Type:       v1alpha1.ClusterSetIP,
			ClusterIPs: []string{"10.0.0.1"},
			Ports: []v1alpha1.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
	},
	"svc6.testns": {
		{
			Name:       "svc6",
			Namespace:  "testns",
			Type:       v1alpha1.ClusterSetIP,
			ClusterIPs: []string{"1234:abcd::1"},
			Ports: []v1alpha1.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
	},
	"hdls1.testns": {
		{
			Name:      "hdls1",
			Namespace: "testns",
			Type:      v1alpha1.Headless,
		},
	},
	"hdlsprtls.testns": {
		{
			Name:      "hdlsprtls",
			Namespace: "testns",
			Type:      v1alpha1.Headless,
		},
	},
	"svc-dual-stack.testns": {
		{
			Name:       "svc-dual-stack",
			Namespace:  "testns",
			Type:       v1alpha1.ClusterSetIP,
			ClusterIPs: []string{"10.0.0.3", "10::3"}, Ports: []v1alpha1.ServicePort{
				{Name: "http", Protocol: "tcp", Port: 80},
			},
		},
	},
}

func (controllerMock2) SvcIndex(s string) []*object.ServiceImport { return svcIndex[s] }

func (controllerMock2) ServiceList() []*object.ServiceImport {
	var svcs []*object.ServiceImport
	for _, svc := range svcIndex {
		svcs = append(svcs, svc...)
	}
	return svcs
}

var epsIndex = map[string][]*object.Endpoints{
	"kubedns.kube-system": {{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "172.0.0.100"},
				},
				Ports: []object.EndpointPort{
					{Port: 53, Protocol: "udp", Name: "dns"},
				},
			},
		},
		ClusterId: "clusterid",
		Name:      "kubedns",
		Namespace: "kube-system",
		Index:     object.EndpointsKey("kubedns", "kube-system"),
	}},
	"svc1.testns": {{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "172.0.0.1", Hostname: "ep1a"},
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		ClusterId: "clusterid",
		Name:      "svc1-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svc1", "testns"),
	}},
	"svcempty.testns": {{
		Subsets: []object.EndpointSubset{
			{
				Addresses: nil,
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		ClusterId: "clusterid",
		Name:      "svcempty-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("svcempty", "testns"),
	}},
	"hdls1.testns": {{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "172.0.0.2"},
					{IP: "172.0.0.3"},
					{IP: "172.0.0.4", Hostname: "dup-name"},
					{IP: "172.0.0.5", Hostname: "dup-name"},
					{IP: "5678:abcd::1"},
					{IP: "5678:abcd::2"},
				},
				Ports: []object.EndpointPort{
					{Port: 80, Protocol: "tcp", Name: "http"},
				},
			},
		},
		ClusterId: "clusterid",
		Name:      "hdls1-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("hdls1", "testns"),
	}},
	"hdlsprtls.testns": {{
		Subsets: []object.EndpointSubset{
			{
				Addresses: []object.EndpointAddress{
					{IP: "172.0.0.20"},
				},
				Ports: []object.EndpointPort{{Port: -1}},
			},
		},
		ClusterId: "clusterid",
		Name:      "hdlsprtls-slice1",
		Namespace: "testns",
		Index:     object.EndpointsKey("hdlsprtls", "testns"),
	}},
}

func (controllerMock2) EpIndex(s string) []*object.Endpoints {
	return epsIndex[s]
}

func (controllerMock2) EndpointsList() []*object.Endpoints {
	var eps []*object.Endpoints
	for _, ep := range epsIndex {
		eps = append(eps, ep...)
	}
	return eps
}

func (controllerMock2) GetNamespaceByName(name string) (*object.Namespace, error) {
	if name == "pod-nons" { // handler_pod_verified_test.go uses this for non-existent namespace.
		return nil, fmt.Errorf("namespace not found")
	}
	if name == "nsnoexist" {
		return nil, fmt.Errorf("namespace not found")
	}
	return &object.Namespace{
		Name: name,
	}, nil
}
