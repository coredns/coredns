package kubernetes

import (
	"slices"
	"testing"

	testhelper "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestAutoPath(t *testing.T) {
	// Set up a Kubernetes object for testing
	defaultZone := "interwebs.test."
	k := New([]string{defaultZone})
	k.autoPathSearch = []string{"custom."}
	k.APIConn = &APIConnServiceTest{}
	k.podMode = podModeVerified
	k.opts.initPodCache = true

	type autopathTest struct {
		qname      string
		searchpath []string
		ip         string
		zone       string
	}
	tests := []autopathTest{
		{
			// Cluster IP Service FQDN - first query
			qname: "svc1.testns.svc.interwebs.test.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Cluster IP Service name only or service inside custom domain - first query
			qname: "svc1.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// External service - first query
			qname: "example.com.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// External service matching zone "." - first query
			qname: "example.com.testns.svc.",
			zone:  ".",
			searchpath: []string{
				"testns.svc.",
				"svc.",
				".",
				"custom.",
				"",
			},
		},
		{
			// External service matching zone "." - first query - host pod
			qname: "example.com.testns.svc.",
			zone:  ".",
			ip:    "10.16.0.1",
			searchpath: []string{
				"testns.svc.",
				"svc.",
				".",
				"custom.",
				"",
			},
		},
		{
			// External service - first query - host pod
			qname: "example.com.other.svc.interwebs.test.",
			ip:    "10.16.0.1",
			searchpath: []string{
				"other.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// External service - second query
			qname: "example.com.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with other namespace in second query - normal pod
			qname: "example.other.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with testns namespace in second query - normal pod
			qname: "example.testns.svc.interwebs.test.",
			searchpath: []string{
				"testns.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
		{
			// Domain conflicting with other namespace in second query - host pod
			qname: "example.other.svc.interwebs.test.",
			ip:    "10.16.0.1",
			searchpath: []string{
				"other.svc.interwebs.test.",
				"svc.interwebs.test.",
				"interwebs.test.",
				"custom.",
				"",
			},
		},
	}

	for _, test := range tests {
		writer := &testhelper.ResponseWriter{}
		if test.ip != "" {
			writer.RemoteIP = test.ip
		}
		zone := "interwebs.test."
		if test.zone != "" {
			zone = test.zone
			k.Zones[0] = test.zone
		}
		state := request.Request{
			Req:  &dns.Msg{Question: []dns.Question{{Name: test.qname, Qtype: dns.TypeA}}},
			Zone: zone, // must match from k.Zones[0]
			W:    writer,
		}
		searchpath := k.AutoPath(state)
		if !slices.Equal(searchpath, test.searchpath) {
			t.Errorf("Error in query %s: expected searchpath %v, but got %v", test.qname, test.searchpath, searchpath)
		}
		if test.zone != "" {
			k.Zones[0] = defaultZone
		}
	}
}
