package kubernetes

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/middleware/etcd/msg"
	"github.com/miekg/dns"
	"k8s.io/client-go/1.5/pkg/api"
)

func testStripFederation(t *testing.T, k Kubernetes, input []string, expectedFed string, expectedSegs string) {
	fed, segs := k.stripFederation(input)

	if expectedSegs != strings.Join(segs, ".") {
		t.Errorf("For '%v', expected segs result '%v'. Instead got result '%v'.", strings.Join(input, "."), expectedSegs, strings.Join(segs, "."))
	}
	if expectedFed != fed {
		t.Errorf("For '%v', expected fed result '%v'. Instead got result '%v'.", strings.Join(input, "."), expectedFed, fed)
	}
}

func TestStripFederation(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs.test"}}
	k.Federations = []Federation{{name: "fed", zone: "era.tion.com"}}

	testStripFederation(t, k, []string{"service", "ns", "fed", "svc"}, "fed", "service.ns.svc")
	testStripFederation(t, k, []string{"service", "ns", "foo", "svc"}, "", "service.ns.foo.svc")
	testStripFederation(t, k, []string{"foo", "bar"}, "", "foo.bar")

}

type apiConnFedTest struct{}

func (apiConnFedTest) Run() {
	return
}

func (apiConnFedTest) Stop() error {
	return nil
}

func (apiConnFedTest) ServiceList() []*api.Service {
	return []*api.Service{}

}

func (apiConnFedTest) PodIndex(string) []interface{} {
	return nil
}

func (apiConnFedTest) EndpointsList() api.EndpointsList {
	return api.EndpointsList{}
}

func (apiConnFedTest) NodeList() api.NodeList {
	return api.NodeList{
		Items: []api.Node{
			api.Node{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{
						LabelAvailabilityZone: "fd-az",
					},
				},
			},
			api.Node{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{
						LabelRegion: "fd-r",
					},
				},
			},
			api.Node{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{
						LabelRegion:           "fd-r",
						LabelAvailabilityZone: "fd-az",
					},
				},
			},
		},
	}
}

func testFederationCNAMERecord(t *testing.T, k Kubernetes, input recordRequest, expected msg.Service) {
	svc := k.federationCNAMERecord(input)

	if expected.Host != svc.Host {
		t.Errorf("For '%v', expected Host result '%v'. Instead got result '%v'.", input, expected.Host, svc.Host)
	}
	if expected.Key != svc.Key {
		t.Errorf("For '%v', expected Key result '%v'. Instead got result '%v'.", input, expected.Key, svc.Key)
	}
}

func TestFederationCNAMERecord(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs"}}
	k.Federations = []Federation{{name: "fed", zone: "era.tion.com"}}
	k.APIConn = apiConnFedTest{}

	var r recordRequest
	r, _ = k.parseRequest("s1.ns.fed.svc.inter.webs", dns.TypeA)
	testFederationCNAMERecord(t, k, r, msg.Service{Key: "/coredns/webs/inter/svc/fed/ns/s1", Host: "s1.ns.fed.svc.fd-az.fd-r.era.tion.com"})

}
