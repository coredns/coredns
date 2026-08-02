package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		query        string
		expected     string // output from r.String()
		multicluster bool
		zonal        bool
		zone         string // expected r.zone
	}{
		// valid SRV request
		{"_http._tcp.webs.mynamespace.svc.inter.webs.tests.", "http.tcp...webs.mynamespace.svc", false, false, ""},
		// A request of endpoint
		{"1-2-3-4.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4..webs.mynamespace.svc", false, false, ""},
		// bare zone
		{"inter.webs.tests.", "......", false, false, ""},
		// bare svc type
		{"svc.inter.webs.tests.", "......", false, false, ""},
		// bare pod type
		{"pod.inter.webs.tests.", "......", false, false, ""},
		// SRV request with empty segments
		{"..webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, false, ""},
		// A multicluster request with a clusterid
		{"1-2-3-4.cluster1.webs.mynamespace.svc.inter.webs.tests.", "..1-2-3-4.cluster1.webs.mynamespace.svc", true, false, ""},
		// zone-scoped name
		{"us-west-2a._zone.webs.mynamespace.svc.inter.webs.tests.", "....webs.mynamespace.svc", false, true, "us-west-2a"},
		// same name without the zonal option reads as port/protocol
		{"us-west-2a._zone.webs.mynamespace.svc.inter.webs.tests.", "us-west-2a.zone...webs.mynamespace.svc", false, false, ""},
		// _zone under the pod subdomain is not the zonal grammar
		{"us-west-2a._zone.webs.mynamespace.pod.inter.webs.tests.", "us-west-2a.zone...webs.mynamespace.pod", false, true, ""},
		// zonal names are not defined in multicluster zones
		{"us-west-2a._zone.webs.mynamespace.svc.inter.webs.tests.", "us-west-2a.zone...webs.mynamespace.svc", true, true, ""},
	}
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		r, e := parseRequest(state.Name(), state.Zone, tc.multicluster, tc.zonal)
		if e != nil {
			t.Errorf("Test %d, expected no error, got '%v'.", i, e)
		}
		rs := r.String()
		if rs != tc.expected {
			t.Errorf("Test %d, expected (stringified) recordRequest: %s, got %s", i, tc.expected, rs)
		}
		if r.zone != tc.zone {
			t.Errorf("Test %d, expected zone %q, got %q", i, tc.zone, r.zone)
		}
	}
}

func TestParseInvalidRequest(t *testing.T) {
	invalid := []string{
		"webs.mynamespace.pood.inter.webs.test.",                 // Request must be for pod or svc subdomain.
		"too.long.for.what.I.am.trying.to.pod.inter.webs.tests.", // Too long.
	}

	for i, query := range invalid {
		m := new(dns.Msg)
		m.SetQuestion(query, dns.TypeA)
		state := request.Request{Zone: zone, Req: m}

		if _, e := parseRequest(state.Name(), state.Zone, false, false); e == nil {
			t.Errorf("Test %d: expected error from %s, got none", i, query)
		}
	}
}

const zone = "inter.webs.tests."
