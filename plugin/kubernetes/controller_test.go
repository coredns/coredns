package kubernetes

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/testutil"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	namespace = "testns"
)

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func BenchmarkController(b *testing.B) {
	client := fake.NewSimpleClientset()
	dco := dnsControlOpts{
		zones: []string{"cluster.local."},
	}
	controller := newdnsController(client, dco)
	cidr := "10.0.0.0/19"

	// Add resources
	generateEndpoints(cidr, client)
	generateSvcs(cidr, "all", client)
	m := new(dns.Msg)
	m.SetQuestion("svc1.testns.svc.cluster.local.", dns.TypeA)
	k := New([]string{"cluster.local."})
	k.APIConn = controller
	ctx := context.Background()
	rw := &test.ResponseWriter{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k.ServeDNS(ctx, rw, m)
	}
}

func TestDnsProgrammingLatency(t *testing.T) {
	client := fake.NewSimpleClientset()
	now := time.Now()
	controller := newdnsController(client, dnsControlOpts{
		initEndpointsCache: true,
		// This is needed as otherwise the fake k8s client doesn't work properly.
		doNotClearObjectsForTests: true,
	})
	controller.durationSinceFunc = func(t time.Time) time.Duration {
		return now.Sub(t)
	}
	DnsProgrammingLatency.Reset()
	go controller.Run()

	createService(t, client, controller, "my-service", api.ClusterIPNone)
	createEndpoints(t, client, "my-service", now.Add(-2*time.Second))
	// Same triggerTime - will be ignored
	updateEndpoints(t, client, "my-service", now.Add(-2*time.Second))
	// Newer trigger time - will be recorded
	updateEndpoints(t, client, "my-service", now.Add(-1*time.Second))

	createEndpoints(t, client, "endpoints-no-service", now.Add(-4*time.Second))

	createService(t, client, controller, "clusterIP-service", "10.40.0.12")
	createEndpoints(t, client, "clusterIP-service", now.Add(-8*time.Second))

	createService(t, client, controller, "headless-no-annotation", api.ClusterIPNone)
	createEndpoints(t, client, "headless-no-annotation", nil)

	createService(t, client, controller, "headless-wrong-annotation", api.ClusterIPNone)
	createEndpoints(t, client, "headless-wrong-annotation", "wrong-value")

	controller.Stop()
	expected := `
        # HELP coredns_kubernetes_dns_programming_latency_seconds Histogram of the time (in seconds) it took to program a dns instance.
        # TYPE coredns_kubernetes_dns_programming_latency_seconds histogram
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.001"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.002"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.004"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.008"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.016"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.032"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.064"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.128"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.256"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="0.512"} 0
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="1.024"} 1
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="2.048"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="4.096"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="8.192"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="16.384"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="32.768"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="65.536"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="131.072"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="262.144"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="524.288"} 2
        coredns_kubernetes_dns_programming_latency_seconds_bucket{service_kind="headless_with_selector",le="+Inf"} 2
        coredns_kubernetes_dns_programming_latency_seconds_sum{service_kind="headless_with_selector"} 3
        coredns_kubernetes_dns_programming_latency_seconds_count{service_kind="headless_with_selector"} 2
	`
	if err := testutil.CollectAndCompare(DnsProgrammingLatency, strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
}

func generateEndpoints(cidr string, client kubernetes.Interface) {
	// https://groups.google.com/d/msg/golang-nuts/zlcYA4qk-94/TWRFHeXJCcYJ
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
	}

	count := 1
	ep := &api.Endpoints{
		Subsets: []api.EndpointSubset{{
			Ports: []api.EndpointPort{
				{
					Port:     80,
					Protocol: "tcp",
					Name:     "http",
				},
			},
		}},
		ObjectMeta: meta.ObjectMeta{
			Namespace: namespace,
		},
	}
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ep.Subsets[0].Addresses = []api.EndpointAddress{
			{
				IP:       ip.String(),
				Hostname: "foo" + strconv.Itoa(count),
			},
		}
		ep.ObjectMeta.Name = "svc" + strconv.Itoa(count)
		_, err = client.Core().Endpoints(namespace).Create(ep)
		count += 1
	}
}

func generateSvcs(cidr string, svcType string, client kubernetes.Interface) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
	}

	count := 1
	switch svcType {
	case "clusterip":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createClusterIPSvc(count, client, ip)
			count += 1
		}
	case "headless":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createHeadlessSvc(count, client, ip)
			count += 1
		}
	case "external":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createExternalSvc(count, client, ip)
			count += 1
		}
	default:
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			if count%3 == 0 {
				createClusterIPSvc(count, client, ip)
			} else if count%3 == 1 {
				createHeadlessSvc(count, client, ip)
			} else if count%3 == 2 {
				createExternalSvc(count, client, ip)
			}
			count += 1
		}
	}
}

func createClusterIPSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.Core().Services(namespace).Create(&api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "svc" + strconv.Itoa(suffix),
			Namespace: namespace,
		},
		Spec: api.ServiceSpec{
			ClusterIP: ip.String(),
			Ports: []api.ServicePort{{
				Name:     "http",
				Protocol: "tcp",
				Port:     80,
			}},
		},
	})
}

func createHeadlessSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.Core().Services(namespace).Create(&api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "hdls" + strconv.Itoa(suffix),
			Namespace: namespace,
		},
		Spec: api.ServiceSpec{
			ClusterIP: api.ClusterIPNone,
		},
	})
}

func createExternalSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.Core().Services(namespace).Create(&api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "external" + strconv.Itoa(suffix),
			Namespace: namespace,
		},
		Spec: api.ServiceSpec{
			ExternalName: "coredns" + strconv.Itoa(suffix) + ".io",
			Ports: []api.ServicePort{{
				Name:     "http",
				Protocol: "tcp",
				Port:     80,
			}},
			Type: api.ServiceTypeExternalName,
		},
	})
}

func buildEndpoints(name string, lastChangeTriggerTime interface{}) *api.Endpoints {
	annotations := make(map[string]string)
	switch v := lastChangeTriggerTime.(type) {
	case string:
		annotations[api.EndpointsLastChangeTriggerTime] = v
	case time.Time:
		annotations[api.EndpointsLastChangeTriggerTime] = v.Format(time.RFC3339Nano)
	}
	return &api.Endpoints{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name, Annotations: annotations},
	}
}

func createEndpoints(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}) {
	_, err := client.CoreV1().Endpoints(namespace).Create(buildEndpoints(name, triggerTime))
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpoints(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}) {
	_, err := client.CoreV1().Endpoints(namespace).Update(buildEndpoints(name, triggerTime))
	if err != nil {
		t.Fatal(err)
	}
}

func createService(t *testing.T, client kubernetes.Interface, controller dnsController, name string, clusterIp string) {
	if _, err := client.CoreV1().Services(namespace).Create(&api.Service{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name},
		Spec: api.ServiceSpec{ ClusterIP: clusterIp },
	}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollImmediate(10*time.Millisecond, 10*time.Second, func() (bool, error) {
		return len(controller.SvcIndex(object.ServiceKey(name, namespace))) == 1, nil
	}); err != nil {
		t.Fatal(err)
	}
}
