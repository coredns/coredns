// +build k8s k8s1

package kubernetes

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/coredns/coredns/middleware/test"

	"github.com/miekg/dns"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var dnsTestCasesA = []test.Case{
	{
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      5    IN      A       10.0.0.100"),
		},
	},
	{
		Qname: "bogusservice.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "bogusendpoint.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "bogusendpoint.headless-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "svc-1-a.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.*.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{
		Qname: "svc-1-a.any.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.any.svc.cluster.local.      303    IN      A       10.0.0.100"),
		},
	},
	{
		Qname: "bogusservice.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "bogusservice.any.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
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
	{
		Qname: "any.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: append([]dns.RR{
			test.A("any.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
			test.A("any.test-1.svc.cluster.local.      303    IN      A       10.0.0.110"),
			test.A("any.test-1.svc.cluster.local.      303    IN      A       10.0.0.115"),
			test.CNAME("any.test-1.svc.cluster.local.  303    IN	     CNAME	  example.net."),
			test.A("example.net.		303	IN	A	13.14.15.16"),
		}, headlessAResponse("any.test-1.svc.cluster.local.", "headless-svc", "test-1")...),
	},
	{
		Qname: "any.test-2.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.test-2.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: append([]dns.RR{
			test.A("*.*.svc.cluster.local.      303    IN      A       10.0.0.100"),
			test.A("*.*.svc.cluster.local.      303    IN      A       10.0.0.110"),
			test.A("*.*.svc.cluster.local.      303    IN      A       10.0.0.115"),
			test.CNAME("*.*.svc.cluster.local.  303    IN	     CNAME	  example.net."),
			test.A("example.net.		303	IN	A	13.14.15.16"),
		}, headlessAResponse("*.*.svc.cluster.local.", "headless-svc", "test-1")...),
	},
	{
		Qname: "headless-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode:  dns.RcodeSuccess,
		Answer: headlessAResponse("headless-svc.test-1.svc.cluster.local.", "headless-svc", "test-1"),
	},
	{
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeServerFailure,
	},
	{
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
