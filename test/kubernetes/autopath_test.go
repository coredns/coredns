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
	{ // query hit on no search domains in path
		Qname: "svc-1-a.test-1.svc.cluster.local", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc-1-a.test-1.svc.cluster.local.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
}

func TestAutopath(t *testing.T) {
	corefile :=
		`    .:53 {
      autopath @kubernetes
      kubernetes cluster.local {
        pods verified
      }
    }
`
	err := loadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	doIntegrationTests(t, autopathTests, "test-1")
	println("BEGIN COREDNS LOGS")
	println(corednsLogs())
	println("END COREDNS LOGS")

}
