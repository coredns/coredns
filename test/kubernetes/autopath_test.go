// +build k8s k8s1 k8sauto

package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var autopathTests = []test.Case{
	{ // query hit on first search domain in path
		Qname: "svc-1-a", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{ // query hit on second search domain in path
		Qname: "svc-1-a.test-1", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc-1-a.test-1.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{ // query hit on third search domain in path
		Qname: "svc-1-a.test-1.svc", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc-1-a.test-1.svc.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
		{ // query hit on no search domains in path (in kubernetes zone)
			Qname: "svc-1-a.test-1.svc.cluster.local", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.CNAME("svc-1-a.test-1.svc.cluster.local.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
				test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
			},
		},
			{ // query hit on no search domains in path (out of kubernetes zone)
				Qname: "foo.example.net", Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.CNAME("foo.example.net.test-1.svc.cluster.local.  303    IN	     CNAME	  foo.example.net."),
					test.A("foo.example.net.      303    IN      A       10.10.10.11"),
				},
				Ns: []dns.RR{
					test.NS("example.net.	303	IN	NS	ns.example.net."),
				},
			},
			{ // query miss
				Qname: "bar.example.net", Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
			},
}

func TestAutopath(t *testing.T) {
	corefile :=
		`    .:53 {
      errors
      log
      autopath @kubernetes
      kubernetes cluster.local {
        pods verified
      }
      file /etc/coredns/Zonefile example.net
    }
`
	exampleZonefile := `    ; example.net test file for autopath tests
    example.net.		IN	SOA	sns.example.netnet. noc.example.net. 2015082541 7200 3600 1209600 3600
    example.net.		IN	NS	ns.example.net.
    example.net.      IN      A	10.10.10.10
    foo.example.net.      IN      A	10.10.10.11
`
	err := loadCorefileAndZonefile(corefile, exampleZonefile)
	if err != nil {
		t.Fatalf("Could not load corefile/zonefile: %s", err)
	}

	doIntegrationTests(t, autopathTests, "test-1")

}
