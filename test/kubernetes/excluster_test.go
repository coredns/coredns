// +build k8s k8sexclust

package kubernetes

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	intTest "github.com/coredns/coredns/test"

	"github.com/miekg/dns"
)

func TestKubernetesAPIFallthrough(t *testing.T) {
	tests := []test.Case{
		{
			Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
			},
		},
	}

	err := exec.Command("kubectl", "proxy", "-p", "8080").Start()
	if err != nil {
		t.Fatalf("Could start kubectl proxy: %s", err)
	}

	certDir := os.Getenv("HOME") + "/.minikube/"
	corefile :=
		`.:0 {
    kubernetes cluster.local {
        endpoint nonexistance:8080,invalidip:8080,localhost:8080
        #endpoint nonexistance:8080,invalidip:8080,https://127.0.0.1:8443
        #tls ` + certDir + `client.crt ` + certDir + `client.key ` + certDir + `ca.crt
    }`

	server, udp, _, err := intTest.CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer server.Stop()

	// Work-around for timing condition that results in no-data being returned in test environment.
	time.Sleep(3 * time.Second)

	for _, tc := range tests {

		c := new(dns.Client)
		m := tc.Msg()

		res, _, err := c.Exchange(m, udp)
		if err != nil {
			t.Fatalf("Could not send query: %s", err)
		}
		test.SortAndCheck(t, res, tc)
	}
}
