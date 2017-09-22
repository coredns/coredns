// +build k8s k8s2

package kubernetes

import (
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var dnsTestCasesFallthrough = []test.Case{
	{
		Qname: "ext-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("example.net.		303	IN	A	13.14.15.16"),
			test.CNAME("ext-svc.test-1.svc.cluster.local. 303 IN	CNAME	example.net."),
		},
	},
	{
		Qname: "f.b.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("f.b.svc.cluster.local.      303    IN      A       10.10.10.11"),
		},
		Ns: []dns.RR{
			test.NS("cluster.local.	303	IN	NS	a.iana-servers.net."),
			test.NS("cluster.local.	303	IN	NS	b.iana-servers.net."),
		},
	},
	{
		Qname: "foo.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("foo.cluster.local.      303    IN      A       10.10.10.10"),
		},
		Ns: []dns.RR{
			test.NS("cluster.local.	303	IN	NS	a.iana-servers.net."),
			test.NS("cluster.local.	303	IN	NS	b.iana-servers.net."),
		},
	},
}

func TestKubernetesFallthrough(t *testing.T) {

	rmFunc, upstream, udp := upstreamServer(t, "example.net", exampleNet)
	defer upstream.Stop()
	defer rmFunc()

	time.Sleep(1 * time.Second)

	corefile := `    .:53 {
	  errors
	  log
      file /etc/coredns/Zonefile cluster.local
      kubernetes cluster.local {
          namespaces test-1
          upstream ` + udp + `
          fallthrough
      }
    }
`
	err := loadCorefileAndZonefile(corefile, clusterLocal)
	if err != nil {
		t.Fatalf("Could not load corefile/zonefile: %s", err)
	}
	doIntegrationTests(t, dnsTestCasesFallthrough, "test-1")
}

const clusterLocal = `    ; cluster.local test file for fallthrough
    cluster.local.		IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600
    cluster.local.		IN	NS	b.iana-servers.net.
    cluster.local.		IN	NS	a.iana-servers.net.
    cluster.local.		IN	A	127.0.0.1
    cluster.local.		IN	A	127.0.0.2
    foo.cluster.local.      IN      A	10.10.10.10
    f.b.svc.cluster.local.  IN      A	10.10.10.11
    *.w.cluster.local.      IN      TXT     "Wildcard"
    a.b.svc.cluster.local.  IN      TXT     "Not a wildcard"
    cname.cluster.local.    IN      CNAME   www.example.net.
    service.namespace.svc.cluster.local.    IN      SRV     8080 10 10 cluster.local.
`
