package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var metadataCases = []struct {
	Qname    string
	Qtype    uint16
	RemoteIP string
	Md       map[string]string
}{
	{
		Qname: "foo.bar.notapod.cluster.local.", Qtype: dns.TypeA,
		Md: map[string]string{
			"kubernetes/parse-error": "invalid query name",
		},
	},
	{
		Qname: "10-240-0-1.podns.pod.cluster.local.", Qtype: dns.TypeA,
		Md: map[string]string{
			"kubernetes/endpoint":  "",
			"kubernetes/kind":      "pod",
			"kubernetes/namespace": "podns",
			"kubernetes/port-name": "",
			"kubernetes/protocol":  "",
			"kubernetes/service":   "10-240-0-1",
		},
	},
	{
		Qname: "s.ns.svc.cluster.local.", Qtype: dns.TypeA,
		Md: map[string]string{
			"kubernetes/endpoint":  "",
			"kubernetes/kind":      "svc",
			"kubernetes/namespace": "ns",
			"kubernetes/port-name": "",
			"kubernetes/protocol":  "",
			"kubernetes/service":   "s",
		},
	},
	{
		Qname: "s.ns.svc.cluster.local.", Qtype: dns.TypeA,
		RemoteIP: "10.10.10.10",
		Md: map[string]string{
			"kubernetes/endpoint":  "",
			"kubernetes/kind":      "svc",
			"kubernetes/namespace": "ns",
			"kubernetes/port-name": "",
			"kubernetes/protocol":  "",
			"kubernetes/service":   "s",
		},
	},
	{
		Qname: "_http._tcp.s.ns.svc.cluster.local.", Qtype: dns.TypeSRV,
		RemoteIP: "10.10.10.10",
		Md: map[string]string{
			"kubernetes/endpoint":  "",
			"kubernetes/kind":      "svc",
			"kubernetes/namespace": "ns",
			"kubernetes/port-name": "http",
			"kubernetes/protocol":  "tcp",
			"kubernetes/service":   "s",
		},
	},
	{
		Qname: "ep.s.ns.svc.cluster.local.", Qtype: dns.TypeA,
		RemoteIP: "10.10.10.10",
		Md: map[string]string{
			"kubernetes/endpoint":  "ep",
			"kubernetes/kind":      "svc",
			"kubernetes/namespace": "ns",
			"kubernetes/port-name": "",
			"kubernetes/protocol":  "",
			"kubernetes/service":   "s",
		},
	},
	{
		Qname: "ep.c1.s.ns.svc.clusterset.local.", Qtype: dns.TypeA,
		RemoteIP: "10.10.10.10",
		Md: map[string]string{
			"kubernetes/cluster":   "c1",
			"kubernetes/endpoint":  "ep",
			"kubernetes/kind":      "svc",
			"kubernetes/namespace": "ns",
			"kubernetes/port-name": "",
			"kubernetes/protocol":  "",
			"kubernetes/service":   "s",
		},
	},
	{
		Qname: "example.com.", Qtype: dns.TypeA,
		RemoteIP: "10.10.10.10",
		Md:       map[string]string{},
	},
}

func mapsDiffer(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}

	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return true
		}
	}
	return false
}

func TestMetadata(t *testing.T) {
	k := New([]string{"cluster.local.", "clusterset.local."})
	k.opts.multiclusterZones = []string{"clusterset.local."}
	k.APIConn = &APIConnServeTest{}

	for i, tc := range metadataCases {
		ctx := metadata.ContextWithMetadata(context.Background())
		zone := "."
		if strings.Contains(tc.Qname, "clusterset.local") {
			zone = "clusterset.local."
		}
		state := request.Request{
			Req:  &dns.Msg{Question: []dns.Question{{Name: tc.Qname, Qtype: tc.Qtype}}},
			Zone: zone,
			W:    &test.ResponseWriter{RemoteIP: tc.RemoteIP},
		}

		k.Metadata(ctx, state)

		md := make(map[string]string)
		for _, l := range metadata.Labels(ctx) {
			md[l] = metadata.ValueFunc(ctx, l)()
		}
		if mapsDiffer(tc.Md, md) {
			t.Errorf("Case %d expected metadata %v and got %v", i, tc.Md, md)
		}
	}
}

func TestMetadataPodsVerified(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.podMode = podModeVerified
	k.APIConn = &APIConnServeTest{}

	ctx := metadata.ContextWithMetadata(context.Background())
	state := request.Request{
		Req:  &dns.Msg{Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA}}},
		Zone: ".",
		W:    &test.ResponseWriter{},
	}

	k.Metadata(ctx, state)

	expect := map[string]string{
		"kubernetes/client-namespace":                    "podns",
		"kubernetes/client-pod-name":                     "foo",
		"kubernetes/client-label/app.kubernetes.io/name": "foo",
		"kubernetes/client-label/bar":                    "baz",
	}

	md := make(map[string]string)
	for _, l := range metadata.Labels(ctx) {
		md[l] = metadata.ValueFunc(ctx, l)()
	}
	if mapsDiffer(expect, md) {
		t.Errorf("Expected metadata %v and got %v", expect, md)
	}
}
