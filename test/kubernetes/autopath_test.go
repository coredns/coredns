// +build k8s k8sAuto

package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var autopathTests = []test.Case{
	{
		Qname: "svc-1-a", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
    {
		Qname: "svc-c.test-2", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.CNAME("svc-c.test-2.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-c.test-2.svc.cluster.local."),
			test.A("svc-c.test-2.svc.cluster.local.      303    IN      A       10.0.0.120"),
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

}
