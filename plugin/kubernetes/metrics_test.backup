package kubernetes

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/prometheus/client_golang/prometheus/testutil"
	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	namespace = "testns"
)

var expected = `
        # HELP coredns_kubernetes_dns_programming_duration_seconds Histogram of the time (in seconds) it took to program a dns instance.
        # TYPE coredns_kubernetes_dns_programming_duration_seconds histogram
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.001"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.002"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.004"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.008"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.016"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.032"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.064"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.128"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.256"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="0.512"} 0
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="1.024"} 1
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="2.048"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="4.096"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="8.192"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="16.384"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="32.768"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="65.536"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="131.072"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="262.144"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="524.288"} 2
        coredns_kubernetes_dns_programming_duration_seconds_bucket{service_kind="headless_with_selector",le="+Inf"} 2
        coredns_kubernetes_dns_programming_duration_seconds_sum{service_kind="headless_with_selector"} 3
        coredns_kubernetes_dns_programming_duration_seconds_count{service_kind="headless_with_selector"} 2
	`

func TestDNSProgrammingLatencyEndpointSlices(t *testing.T) {
	now := time.Now()

	svcIdx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{svcNameNamespaceIndex: svcNameNamespaceIndexFunc})
	epIdx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})

	dns := dnsControl{svcLister: svcIdx}
	svcProc := object.DefaultProcessor(object.ToService, nil)(svcIdx, cache.ResourceEventHandlerFuncs{})
	epProc := object.DefaultProcessor(object.EndpointSliceToEndpoints, dns.EndpointSliceLatencyRecorder())(epIdx, cache.ResourceEventHandlerFuncs{})

	object.DurationSinceFunc = func(t time.Time) time.Duration {
		return now.Sub(t)
	}
	object.DNSProgrammingLatency.Reset()

	endpoints1 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.4"},
	}}

	endpoints2 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.45"},
	}}

	createService(t, svcProc, "my-service", api.ClusterIPNone)
	createEndpointSlice(t, epProc, "my-service", now.Add(-2*time.Second), endpoints1)
	updateEndpointSlice(t, epProc, "my-service", now.Add(-1*time.Second), endpoints2)

	createEndpointSlice(t, epProc, "endpoints-no-service", now.Add(-4*time.Second), nil)

	createService(t, svcProc, "clusterIP-service", "10.40.0.12")
	createEndpointSlice(t, epProc, "clusterIP-service", now.Add(-8*time.Second), nil)

	createService(t, svcProc, "headless-no-annotation", api.ClusterIPNone)
	createEndpointSlice(t, epProc, "headless-no-annotation", nil, nil)

	createService(t, svcProc, "headless-wrong-annotation", api.ClusterIPNone)
	createEndpointSlice(t, epProc, "headless-wrong-annotation", "wrong-value", nil)

	if err := testutil.CollectAndCompare(object.DNSProgrammingLatency, strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
}

func TestDnsProgrammingLatencyEndpoints(t *testing.T) {
	now := time.Now()

	svcIdx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{svcNameNamespaceIndex: svcNameNamespaceIndexFunc})
	epIdx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})

	dns := dnsControl{svcLister: svcIdx}
	svcProc := object.DefaultProcessor(object.ToService, nil)(svcIdx, cache.ResourceEventHandlerFuncs{})
	epProc := object.DefaultProcessor(object.ToEndpoints, dns.EndpointsLatencyRecorder())(epIdx, cache.ResourceEventHandlerFuncs{})

	object.DurationSinceFunc = func(t time.Time) time.Duration {
		return now.Sub(t)
	}
	object.DNSProgrammingLatency.Reset()

	subset1 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
	}}

	subset2 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.5", Hostname: "foo"}},
	}}

	createService(t, svcProc, "my-service", api.ClusterIPNone)
	createEndpoints(t, epProc, "my-service", now.Add(-2*time.Second), subset1)
	updateEndpoints(t, epProc, "my-service", now.Add(-1*time.Second), subset2)

	createEndpoints(t, epProc, "endpoints-no-service", now.Add(-4*time.Second), nil)

	createService(t, svcProc, "clusterIP-service", "10.40.0.12")
	createEndpoints(t, epProc, "clusterIP-service", now.Add(-8*time.Second), nil)

	createService(t, svcProc, "headless-no-annotation", api.ClusterIPNone)
	createEndpoints(t, epProc, "headless-no-annotation", nil, nil)

	createService(t, svcProc, "headless-wrong-annotation", api.ClusterIPNone)
	createEndpoints(t, epProc, "headless-wrong-annotation", "wrong-value", nil)

	if err := testutil.CollectAndCompare(object.DNSProgrammingLatency, strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
}

func buildEndpoints(name string, lastChangeTriggerTime interface{}, subsets []api.EndpointSubset) *api.Endpoints {
	annotations := make(map[string]string)
	switch v := lastChangeTriggerTime.(type) {
	case string:
		annotations[api.EndpointsLastChangeTriggerTime] = v
	case time.Time:
		annotations[api.EndpointsLastChangeTriggerTime] = v.Format(time.RFC3339Nano)
	}
	return &api.Endpoints{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name, Annotations: annotations},
		Subsets:    subsets,
	}
}

func buildEndpointSlice(name string, lastChangeTriggerTime interface{}, endpoints []discovery.Endpoint) *discovery.EndpointSlice {
	annotations := make(map[string]string)
	switch v := lastChangeTriggerTime.(type) {
	case string:
		annotations[api.EndpointsLastChangeTriggerTime] = v
	case time.Time:
		annotations[api.EndpointsLastChangeTriggerTime] = v.Format(time.RFC3339Nano)
	}
	return &discovery.EndpointSlice{
		ObjectMeta: meta.ObjectMeta{
			Namespace: namespace, Name: name + "-12345",
			Labels:      map[string]string{discovery.LabelServiceName: name},
			Annotations: annotations,
		},
		Endpoints: endpoints,
	}
}

func createEndpoints(t *testing.T, processor cache.ProcessFunc, name string, triggerTime interface{}, subsets []api.EndpointSubset) {
	err := processor(cache.Deltas{{Type: cache.Added, Object: buildEndpoints(name, triggerTime, subsets)}})
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpoints(t *testing.T, processor cache.ProcessFunc, name string, triggerTime interface{}, subsets []api.EndpointSubset) {
	err := processor(cache.Deltas{{Type: cache.Updated, Object: buildEndpoints(name, triggerTime, subsets)}})
	if err != nil {
		t.Fatal(err)
	}
}

func createEndpointSlice(t *testing.T, processor cache.ProcessFunc, name string, triggerTime interface{}, endpoints []discovery.Endpoint) {
	err := processor(cache.Deltas{{Type: cache.Added, Object: buildEndpointSlice(name, triggerTime, endpoints)}})
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpointSlice(t *testing.T, processor cache.ProcessFunc, name string, triggerTime interface{}, endpoints []discovery.Endpoint) {
	err := processor(cache.Deltas{{Type: cache.Updated, Object: buildEndpointSlice(name, triggerTime, endpoints)}})
	if err != nil {
		t.Fatal(err)
	}
}

func createService(t *testing.T, processor cache.ProcessFunc, name string, clusterIp string) {
	obj := &api.Service{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       api.ServiceSpec{ClusterIP: clusterIp},
	}
	err := processor(cache.Deltas{{Type: cache.Added, Object: obj}})
	if err != nil {
		t.Fatal(err)
	}
}
