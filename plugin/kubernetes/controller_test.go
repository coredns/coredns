package kubernetes

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
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
	for eip := ip.Mask(ipnet.Mask); ipnet.Contains(eip); inc(eip) {
		ep.Subsets[0].Addresses = []api.EndpointAddress{
			{
				IP:       eip.String(),
				Hostname: "foo" + strconv.Itoa(count),
			},
		}
		ep.ObjectMeta.Name = "svc" + strconv.Itoa(count)
		client.CoreV1().Endpoints("testns").Create(ep)
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
		for cip := ip.Mask(ipnet.Mask); ipnet.Contains(cip); inc(cip) {
			createClusterIPSvc(count, client, cip)
			count++
		}
	case "headless":
		for hip := ip.Mask(ipnet.Mask); ipnet.Contains(hip); inc(hip) {
			createHeadlessSvc(count, client, hip)
			count++
		}
	case "external":
		for eip := ip.Mask(ipnet.Mask); ipnet.Contains(eip); inc(eip) {
			createExternalSvc(count, client, eip)
			count++
		}
	default:
		for dip := ip.Mask(ipnet.Mask); ipnet.Contains(dip); inc(dip) {
			if count%3 == 0 {
				createClusterIPSvc(count, client, dip)
			} else if count%3 == 1 {
				createHeadlessSvc(count, client, dip)
			} else if count%3 == 2 {
				createExternalSvc(count, client, dip)
			}
			count++
		}
	}
}

func createClusterIPSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.CoreV1().Services("testns").Create(&api.Service{
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
	})
}

func createHeadlessSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.CoreV1().Services("testns").Create(&api.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      "hdls" + strconv.Itoa(suffix),
			Namespace: "testns",
		},
		Spec: api.ServiceSpec{
			ClusterIP: api.ClusterIPNone,
		},
	})
}

func createExternalSvc(suffix int, client kubernetes.Interface, ip net.IP) {
	client.CoreV1().Services("testns").Create(&api.Service{
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
	})
}
