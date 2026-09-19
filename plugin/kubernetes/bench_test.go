package kubernetes

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

// The fixtures behind BenchmarkServices and BenchmarkServicesHeadless give every
// endpoint address a single port, which is not what a headless service in a real
// cluster looks like. A StatefulSet or a Deployment behind a headless service
// commonly has tens to hundreds of ready endpoints spread over several
// EndpointSlices, each exposing the handful of ports its containers declare, and
// no per-address Hostname unless the workload sets one.
//
// APIConnBench models that: one headless service with benchEndpoints addresses
// spread over benchSlices slices, each subset exposing benchPorts ports. It is
// the shape that makes the per-port work in the endpoint loops visible.
const (
	benchEndpoints = 100
	benchSlices    = 4
	benchPorts     = 3
)

type APIConnBench struct {
	APIConnServiceTest
}

func (APIConnBench) SvcIndex(string) []*object.Service { return benchServices }
func (APIConnBench) ServiceList() []*object.Service    { return benchServices }
func (APIConnBench) EpIndex(string) []*object.Endpoints {
	return benchEndpointSlices
}
func (APIConnBench) EndpointsList() []*object.Endpoints { return benchEndpointSlices }

var benchServices = []*object.Service{
	{
		Name:       "hdlsbench",
		Namespace:  "testns",
		ClusterIPs: []string{api.ClusterIPNone},
	},
}

var benchEndpointSlices = makeBenchEndpoints()

// makeBenchEndpoints spreads benchEndpoints addresses over benchSlices slices,
// each subset carrying benchPorts ports. The addresses carry no Hostname, so
// endpointHostname falls through to rewriting the IP, which is what an
// EndpointSlice for a plain Deployment looks like.
func makeBenchEndpoints() []*object.Endpoints {
	ports := make([]object.EndpointPort, 0, benchPorts)
	for _, p := range []struct {
		name string
		port int32
	}{{"http", 80}, {"https", 443}, {"metrics", 9153}} {
		ports = append(ports, object.EndpointPort{Port: p.port, Protocol: "tcp", Name: p.name})
	}

	eps := make([]*object.Endpoints, 0, benchSlices)
	per := benchEndpoints / benchSlices
	for s := range benchSlices {
		addrs := make([]object.EndpointAddress, 0, per)
		for i := range per {
			n := s*per + i
			addrs = append(addrs, object.EndpointAddress{IP: fmt.Sprintf("172.16.%d.%d", n/256, n%256)})
		}
		eps = append(eps, &object.Endpoints{
			Subsets:   []object.EndpointSubset{{Addresses: addrs, Ports: ports}},
			Name:      fmt.Sprintf("hdlsbench-slice%d", s),
			Namespace: "testns",
			Index:     object.EndpointsKey("hdlsbench", "testns"),
		})
	}
	return eps
}

func benchKubernetes() *Kubernetes {
	k := New([]string{"inter.webs.tests."})
	k.APIConn = APIConnBench{}
	return k
}

func benchState(k *Kubernetes, qname string, qtype uint16) request.Request {
	m := new(dns.Msg)
	m.SetQuestion(qname, qtype)
	return request.Request{Zone: k.Zones[0], Req: m}
}

// BenchmarkServicesHeadlessMultiPort resolves every endpoint of a headless
// service that exposes several ports, so the answer is benchEndpoints*benchPorts
// records and the endpoint loops run their full length.
func BenchmarkServicesHeadlessMultiPort(b *testing.B) {
	k := benchKubernetes()
	ctx := context.TODO()
	state := benchState(k, "hdlsbench.testns.svc.inter.webs.tests.", dns.TypeA)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = k.Services(ctx, state, false, plugin.Options{})
	}
}

// BenchmarkServicesEndpointMultiPort resolves a single endpoint of that service,
// which walks every address to find the match rather than emitting them all.
func BenchmarkServicesEndpointMultiPort(b *testing.B) {
	k := benchKubernetes()
	ctx := context.TODO()
	state := benchState(k, "172-16-0-77.hdlsbench.testns.svc.inter.webs.tests.", dns.TypeA)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = k.Services(ctx, state, false, plugin.Options{})
	}
}

// BenchmarkServicesSRVMultiPort resolves one named port across every endpoint.
func BenchmarkServicesSRVMultiPort(b *testing.B) {
	k := benchKubernetes()
	ctx := context.TODO()
	state := benchState(k, "_http._tcp.hdlsbench.testns.svc.inter.webs.tests.", dns.TypeSRV)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = k.Services(ctx, state, false, plugin.Options{})
	}
}

// TestBenchFixtureSanity guards the benchmarks above: a fixture that stops
// matching the query would leave them measuring an empty result set rather
// than the endpoint loops they exist to measure.
func TestBenchFixtureSanity(t *testing.T) {
	k := benchKubernetes()
	ctx := context.TODO()
	for _, tc := range []struct {
		name  string
		qname string
		qtype uint16
		want  int
	}{
		{"headless, every port", "hdlsbench.testns.svc.inter.webs.tests.", dns.TypeA, benchEndpoints * benchPorts},
		{"one endpoint", "172-16-0-77.hdlsbench.testns.svc.inter.webs.tests.", dns.TypeA, benchPorts},
		{"SRV, one named port", "_http._tcp.hdlsbench.testns.svc.inter.webs.tests.", dns.TypeSRV, benchEndpoints},
	} {
		svcs, err := k.Services(ctx, benchState(k, tc.qname, tc.qtype), false, plugin.Options{})
		if err != nil {
			t.Fatalf("%s: %s", tc.name, err)
		}
		if len(svcs) != tc.want {
			t.Errorf("%s: got %d services, want %d", tc.name, len(svcs), tc.want)
		}
	}
}
