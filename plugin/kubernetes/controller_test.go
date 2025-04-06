package kubernetes

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	mcsClientsetFake "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned/fake"
	mcsClientset "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned/typed/apis/v1alpha1"
)

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func kubernetesWithFakeClient(ctx context.Context, cidr string, initEndpointsCache bool, svcType string) *Kubernetes {
	client := fake.NewSimpleClientset()
	mcsClient := mcsClientsetFake.NewSimpleClientset()
	dco := dnsControlOpts{
		zones:              []string{"cluster.local.", "clusterset.local."},
		multiclusterZones:  []string{"clusterset.local."},
		initEndpointsCache: initEndpointsCache,
	}
	controller := newdnsController(ctx, client, mcsClient.MulticlusterV1alpha1(), dco)

	// Add resources
	_, err := client.CoreV1().Namespaces().Create(ctx, &api.Namespace{ObjectMeta: meta.ObjectMeta{Name: "testns"}}, meta.CreateOptions{})
	if err != nil {
		log.Fatal(err)
	}
	generateSvcs(cidr, svcType, client, mcsClient.MulticlusterV1alpha1())
	generateEndpointSlices(cidr, svcType, client)
	k := New([]string{"cluster.local.", "clusterset.local."})
	k.APIConn = controller
	k.opts.multiclusterZones = []string{"clusterset.local."}
	return k
}

func BenchmarkController(b *testing.B) {
	ctx := context.Background()
	k := kubernetesWithFakeClient(ctx, "10.0.0.0/24", true, "all")

	go k.APIConn.Run()
	defer k.APIConn.Stop()
	for !k.APIConn.HasSynced() {
		time.Sleep(time.Millisecond)
	}

	rw := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("svc1.testns.svc.cluster.local.", dns.TypeA)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k.ServeDNS(ctx, rw, m)
	}
}

func TestEndpointsDisabled(t *testing.T) {
	ctx := context.Background()
	k := kubernetesWithFakeClient(ctx, "10.0.0.0/30", false, "headless")
	k.opts.initEndpointsCache = false

	go k.APIConn.Run()
	defer k.APIConn.Stop()
	for !k.APIConn.HasSynced() {
		time.Sleep(time.Millisecond)
	}

	rw := &dnstest.Recorder{ResponseWriter: &test.ResponseWriter{}}
	m := new(dns.Msg)
	m.SetQuestion("svc2.testns.svc.cluster.local.", dns.TypeA)
	k.ServeDNS(ctx, rw, m)
	if rw.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %v", dns.RcodeToString[rw.Msg.Rcode])
	}
}

func TestEndpointsEnabled(t *testing.T) {
	ctx := context.Background()
	k := kubernetesWithFakeClient(ctx, "10.0.0.0/30", true, "headless")
	k.opts.initEndpointsCache = true

	go k.APIConn.Run()
	defer k.APIConn.Stop()
	for !k.APIConn.HasSynced() {
		time.Sleep(time.Millisecond)
	}

	rw := &dnstest.Recorder{ResponseWriter: &test.ResponseWriter{}}
	m := new(dns.Msg)
	m.SetQuestion("svc2.testns.svc.cluster.local.", dns.TypeA)
	k.ServeDNS(ctx, rw, m)
	if rw.Msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected SUCCESS, got %v", dns.RcodeToString[rw.Msg.Rcode])
	}
}

func TestMultiClusterHeadless(t *testing.T) {
	ctx := context.Background()
	k := kubernetesWithFakeClient(ctx, "10.0.0.0/30", true, "mcs-headless")
	k.opts.initEndpointsCache = true

	go k.APIConn.Run()
	defer k.APIConn.Stop()
	for !k.APIConn.HasSynced() {
		time.Sleep(time.Millisecond)
	}

	rw := &dnstest.Recorder{ResponseWriter: &test.ResponseWriter{}}
	m := new(dns.Msg)
	m.SetQuestion("svc2.testns.svc.clusterset.local.", dns.TypeA)
	k.ServeDNS(ctx, rw, m)
	if rw.Msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected SUCCESS, got %v", dns.RcodeToString[rw.Msg.Rcode])
	}
}

func generateEndpointSlices(cidr string, svcType string, client kubernetes.Interface) {
	// https://groups.google.com/d/msg/golang-nuts/zlcYA4qk-94/TWRFHeXJCcYJ
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
	}

	count := 1
	port := int32(80)
	protocol := api.Protocol("tcp")
	name := "http"
	eps := &discovery.EndpointSlice{
		Ports: []discovery.EndpointPort{
			{
				Port:     &port,
				Protocol: &protocol,
				Name:     &name,
			},
		},
		ObjectMeta: meta.ObjectMeta{
			Namespace: "testns",
		},
	}
	ctx := context.TODO()
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		hostname := "foo" + strconv.Itoa(count)
		eps.Endpoints = []discovery.Endpoint{
			{
				Addresses: []string{ip.String()},
				Hostname:  &hostname,
			},
		}
		eps.Name = "svc" + strconv.Itoa(count)
		if !strings.Contains(svcType, "mcs") {
			eps.Labels = map[string]string{discovery.LabelServiceName: eps.Name}
		} else {
			eps.Labels = map[string]string{mcs.LabelServiceName: eps.Name}
		}
		_, err := client.DiscoveryV1().EndpointSlices("testns").Create(ctx, eps, meta.CreateOptions{})
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
}

func generateSvcs(cidr string, svcType string, client kubernetes.Interface, mcsClient mcsClientset.MulticlusterV1alpha1Interface) {
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
	case "mcs-headless":
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			createMultiClusterHeadlessSvc(count, mcsClient, ip)
			count++
		}
	default:
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			switch count % 3 {
			case 0:
				createClusterIPSvc(count, client, ip)
			case 1:
				createHeadlessSvc(count, client, ip)
			case 2:
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
			Name:      "svc" + strconv.Itoa(suffix),
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
			Name:      "svc" + strconv.Itoa(suffix),
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

func createMultiClusterHeadlessSvc(suffix int, mcsClient mcsClientset.MulticlusterV1alpha1Interface, ip net.IP) {
	ctx := context.TODO()
	mcsClient.ServiceImports("testns").Create(ctx, &mcs.ServiceImport{
		ObjectMeta: meta.ObjectMeta{
			Name:      "svc" + strconv.Itoa(suffix),
			Namespace: "testns",
		},
		Spec: mcs.ServiceImportSpec{
			Ports: []mcs.ServicePort{{
				Name:     "http",
				Protocol: "tcp",
				Port:     80,
			}},
			Type: mcs.Headless,
		},
	}, meta.CreateOptions{})
}

func TestServiceModified(t *testing.T) {
	tests := []struct {
		oldSvc   interface{}
		newSvc   interface{}
		ichanged bool
		echanged bool
	}{
		{
			oldSvc:   nil,
			newSvc:   &object.Service{},
			ichanged: true,
			echanged: false,
		},
		{
			oldSvc:   &object.Service{},
			newSvc:   nil,
			ichanged: true,
			echanged: false,
		},
		{
			oldSvc:   nil,
			newSvc:   &object.Service{ExternalIPs: []string{"10.0.0.1"}},
			ichanged: true,
			echanged: true,
		},
		{
			oldSvc:   &object.Service{ExternalIPs: []string{"10.0.0.1"}},
			newSvc:   nil,
			ichanged: true,
			echanged: true,
		},
		{
			oldSvc:   &object.Service{ExternalIPs: []string{"10.0.0.1"}},
			newSvc:   &object.Service{ExternalIPs: []string{"10.0.0.2"}},
			ichanged: false,
			echanged: true,
		},
		{
			oldSvc:   &object.Service{ExternalName: "10.0.0.1"},
			newSvc:   &object.Service{ExternalName: "10.0.0.2"},
			ichanged: true,
			echanged: false,
		},
		{
			oldSvc:   &object.Service{Ports: []api.ServicePort{{Name: "test1"}}},
			newSvc:   &object.Service{Ports: []api.ServicePort{{Name: "test2"}}},
			ichanged: true,
			echanged: true,
		},
		{
			oldSvc:   &object.Service{Ports: []api.ServicePort{{Name: "test1"}}},
			newSvc:   &object.Service{Ports: []api.ServicePort{{Name: "test2"}, {Name: "test3"}}},
			ichanged: true,
			echanged: true,
		},
	}

	for i, test := range tests {
		ichanged, echanged := serviceModified(test.oldSvc, test.newSvc)
		if test.ichanged != ichanged || test.echanged != echanged {
			t.Errorf("Expected %v, %v for test %v. Got %v, %v", test.ichanged, test.echanged, i, ichanged, echanged)
		}
	}
}
