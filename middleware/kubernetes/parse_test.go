package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestParsePodOrSvc(t *testing.T) {

	state := request.Request{}
	state.Zone = "cluster.local."
	state.Req = new(dns.Msg)
	k := &Kubernetes{}

	tests := []struct {
		qname string
		what  string
		rcode int
	}{
		{"pod.cluster.local.", "pod", dns.RcodeSuccess},
		{"svc.cluster.local.", "svc", dns.RcodeSuccess},
		{"a.b.svc.cluster.local.", "svc", dns.RcodeSuccess},
		// failures
		{"nopod.cluster.local.", "", dns.RcodeNameError},
	}
	for i, tc := range tests {
		state.Req.SetQuestion(tc.qname, dns.TypeSRV)

		rr, rcode := k.parse(state)
		if rcode != tc.rcode {
			t.Errorf("Test %d, expected %d for rcode, got %d", i, tc.rcode, rcode)
		}
		if rr.podOrSvc != tc.what {
			t.Errorf("Test %d, expected %s for podOrSvc, got %s", i, tc.what, rr.podOrSvc)
		}
	}
}
