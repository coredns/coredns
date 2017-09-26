// +build k8s k8s1

package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesAllNSExposed = []test.Case{
	{
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{
		Qname: "svc-c.test-2.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-c.test-2.svc.cluster.local.      303    IN      A       10.0.0.120"),
		},
	},
}

func TestKubernetesNSExposed(t *testing.T) {
	corefile :=
		`    .:53 {
      errors
      log
      kubernetes cluster.local
    }
`
	err := loadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	doIntegrationTests(t, dnsTestCasesAllNSExposed, "test-1")
}
