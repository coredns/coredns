// +build k8s k8s1

package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesA = []test.Case{
	{ // An A record query for an existing service should return a record
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      5    IN      A       10.0.0.100"),
		},
	},
	{ // An A record query for an existing headless service should return a record for each of its endpoints
		Qname: "headless-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode:  dns.RcodeSuccess,
		Answer: headlessAResponse("headless-svc.test-1.svc.cluster.local.", "headless-svc", "test-1"),
	},
	{ // An A record query for a non-existing service should return NXDOMAIN
		Qname: "bogusservice.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // An A record query for a non-existing endpoint should return NXDOMAIN
		Qname: "bogusendpoint.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // A service record search with a wild card namespace should return all (1) services in exposed namespaces
		Qname: "svc-1-a.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.*.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{ // A wild card service name in an exposed namespace should result in all records
		Qname: "*.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: append([]dns.RR{
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.0.0.110"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.0.0.115"),
			test.CNAME("*.test-1.svc.cluster.local.  303    IN	     CNAME	  example.net."),
			test.A("example.net.		303	IN	A	13.14.15.16"),
		}, headlessAResponse("*.test-1.svc.cluster.local.", "headless-svc", "test-1")...),
	},
	{ // A wild card service name in an un-exposed namespace result in nxdomain
		Qname: "*.test-2.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // By default, pod queries are disabled, so a pod query should return a server failure
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeServerFailure,
	},
	{ // A TXT request for dns-version should return the version of the kubernetes service discovery spec implemented
		Qname: "dns-version.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.TXT(`dns-version.cluster.local. 303 IN TXT "1.0.1"`),
		},
	},
}

func TestKubernetesA(t *testing.T) {

	rmFunc, upstream, udp := upstreamServer(t, "example.net", exampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
            namespaces test-1
            upstream ` + udp + `
        }
    }
`

	err := loadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	doIntegrationTests(t, dnsTestCasesA, "test-1")
}
