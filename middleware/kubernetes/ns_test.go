package kubernetes

import "testing"
import "net"

import "github.com/coredns/coredns/middleware/etcd/msg"

func TestRecordForNS(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs.test"}}
	corednsRecord.Hdr.Name = "coredns.kube-system."
	corednsRecord.A = net.IP("1.2.3.4")
	r, _ := k.parseRequest("inter.webs.test", "NS")
	expected := "/coredns/test/webs/inter/kube-system/coredns"

	var svcs []msg.Service
	k.recordsForNS(r, &svcs)
	if svcs[0].Key != expected {
		t.Errorf("Expected  result '%v'. Instead got result '%v'.", expected, svcs[0].Key)
	}
}

func TestDefaultNSMsg(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs.test"}}
	corednsRecord.Hdr.Name = "coredns.kube-system."
	corednsRecord.A = net.IP("1.2.3.4")
	r, _ := k.parseRequest("ns.dns.inter.webs.test", "A")
	expected := "/coredns/test/webs/inter/dns/ns"

	svc := k.DefaultNSMsg(r)
	if svc.Key != expected {
		t.Errorf("Expected  result '%v'. Instead got result '%v'.", expected, svc.Key)
	}
}

func TestIsDefaultNS(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs.test"}}
	r, _ := k.parseRequest("ns.dns.inter.webs.test", "A")

	var name string
	var expected bool

	name = "ns.dns.inter.webs.test"
	expected = true
	if IsDefaultNS(name, r) != expected {
		t.Errorf("Expected IsDefaultNS('%v') to be '%v'.", name, expected)
	}
	name = "ns.dns.blah.inter.webs.test"
	expected = false
	if IsDefaultNS(name, r) != expected {
		t.Errorf("Expected IsDefaultNS('%v') to be '%v'.", name, expected)
	}
}
