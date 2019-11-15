package kubernetes

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEndpointsEquivalent(t *testing.T) {
	epA := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
		}},
	}
	epB := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
		}},
	}
	epC := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.5", Hostname: "foo"}},
		}},
	}
	epD := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.5", Hostname: "foo"}},
		},
			{
				Addresses: []object.EndpointAddress{{IP: "1.2.2.2", Hostname: "foofoo"}},
			}},
	}
	epE := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.5", Hostname: "foo"}, {IP: "1.1.1.1"}},
		}},
	}
	epF := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foofoo"}},
		}},
	}
	epG := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
			Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "TCP"}},
		}},
	}
	epH := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
			Ports:     []object.EndpointPort{{Name: "newportname", Port: 80, Protocol: "TCP"}},
		}},
	}
	epI := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
			Ports:     []object.EndpointPort{{Name: "http", Port: 8080, Protocol: "TCP"}},
		}},
	}
	epJ := object.Endpoints{
		Subsets: []object.EndpointSubset{{
			Addresses: []object.EndpointAddress{{IP: "1.2.3.4", Hostname: "foo"}},
			Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "UDP"}},
		}},
	}

	tests := []struct {
		equiv bool
		a     *object.Endpoints
		b     *object.Endpoints
	}{
		{true, &epA, &epB},
		{false, &epA, &epC},
		{false, &epA, &epD},
		{false, &epA, &epE},
		{false, &epA, &epF},
		{false, &epF, &epG},
		{false, &epG, &epH},
		{false, &epG, &epI},
		{false, &epG, &epJ},
	}

	for i, tc := range tests {
		if tc.equiv && !endpointsEquivalent(tc.a, tc.b) {
			t.Errorf("Test %d: expected endpoints to be equivalent and they are not.", i)
		}
		if !tc.equiv && endpointsEquivalent(tc.a, tc.b) {
			t.Errorf("Test %d: expected endpoints to be seen as different but they were not.", i)
		}
	}
}

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
	ctx := context.Background()
	controller := newdnsController(ctx, client, dco)
	cidr := "10.0.0.0/19"

	// Add resources
	generateEndpoints(cidr, client)
	generateSvcs(cidr, "all", client)
	m := new(dns.Msg)
	m.SetQuestion("svc1.testns.svc.cluster.local.", dns.TypeA)
	k := New([]string{"cluster.local."})
	k.APIConn = controller
	rw := &test.ResponseWriter{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k.ServeDNS(ctx, rw, m)
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
			Namespace: "testns",
		},
	}
	ctx := context.TODO()
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ep.Subsets[0].Addresses = []api.EndpointAddress{
			{
				IP:       ip.String(),
				Hostname: "foo" + strconv.Itoa(count),
			},
		}
		ep.ObjectMeta.Name = "svc" + strconv.Itoa(count)
		client.CoreV1().Endpoints("testns").Create(ctx, ep, meta.CreateOptions{})
		count++
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
			count++
		}
	case "headless":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createHeadlessSvc(count, client, ip)
			count++
		}
	case "external":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createExternalSvc(count, client, ip)
			count++
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
			count++
		}
	}
}

func createClusterIPSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	ctx := context.TODO()
	client.CoreV1().Services("testns").Create(ctx, &api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "svc" + strconv.Itoa(suffix),
			Namespace: "testns",
		},
		Spec: api.ServiceSpec{
			ClusterIP: ip.String(),
			Ports: []api.ServicePort{{
				Name:     "http",
				Protocol: "tcp",
				Port:     80,
			}},
		},
	}, meta.CreateOptions{})
}

func createHeadlessSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	ctx := context.TODO()
	client.CoreV1().Services("testns").Create(ctx, &api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "hdls" + strconv.Itoa(suffix),
			Namespace: "testns",
		},
		Spec: api.ServiceSpec{
			ClusterIP: api.ClusterIPNone,
		},
	}, meta.CreateOptions{})
}

func createExternalSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	ctx := context.TODO()
	client.CoreV1().Services("testns").Create(ctx, &api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "external" + strconv.Itoa(suffix),
			Namespace: "testns",
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
	}, meta.CreateOptions{})
}
