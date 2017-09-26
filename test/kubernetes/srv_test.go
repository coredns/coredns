// +build k8s k8s2

package kubernetes

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var dnsTestCasesSRV = []test.Case{
	{
		Qname: "*._TcP.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*._TcP.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	 0  50  443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._TcP.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	 0  50   80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303 	IN	A	10.0.0.100"),
		},
	},
	{
		Qname: "*.*.bogusservice.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.any.svc-1-a.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*.any.svc-1-a.*.svc.cluster.local.      303    IN    SRV 0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*.any.svc-1-a.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.0.0.100"),
		},
	},
	{
		Qname: "ANY.*.svc-1-a.any.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("ANY.*.svc-1-a.any.svc.cluster.local.      303    IN    SRV 0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("ANY.*.svc-1-a.any.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303 	IN	A	10.0.0.100"),
		},
	},
	{
		Qname: "*.*.bogusservice.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.*.bogusservice.any.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "_c-port._UDP.*.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: append(srvResponse("_c-port._UDP.*.test-1.svc.cluster.local.", dns.TypeSRV, "headless-svc", "test-1"),
			[]dns.RR{
				test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.      303    IN    SRV 0 33 1234 svc-c.test-1.svc.cluster.local.")}...),
		Extra: append(srvResponse("_c-port._UDP.*.test-1.svc.cluster.local.", dns.TypeA, "headless-svc", "test-1"),
			[]dns.RR{
				test.A("svc-c.test-1.svc.cluster.local.	303	IN	A	10.0.0.115")}...),
	},
	{
		Qname: "*._tcp.any.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 80  svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 80  svc-1-b.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.0.0.100"),
			test.A("svc-1-b.test-1.svc.cluster.local.	303	IN	A	10.0.0.110"),
		},
	},
	{
		Qname: "*.*.any.test-2.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.*.*.test-2.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "_http._tcp.*.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.*.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("_http._tcp.*.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-b.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.0.0.100"),
			test.A("svc-1-b.test-1.svc.cluster.local.	303	IN	A	10.0.0.110"),
		},
	},
	{
		Qname: "*.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode:  dns.RcodeSuccess,
		Answer: srvResponse("*.svc-1-a.test-1.svc.cluster.local.", dns.TypeSRV, "svc-1-a", "test-1"),
		Extra:  srvResponse("*.svc-1-a.test-1.svc.cluster.local.", dns.TypeA, "svc-1-a", "test-1"),
	},
	{
		Qname: "*._not-udp-or-tcp.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.0.0.100"),
		},
	},
}

func TestKubernetesSRV(t *testing.T) {

	rmFunc, upstream, udp := upstreamServer(t, "example.net", exampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
	    errors
	    log
        kubernetes cluster.local {
            namespaces test-1
            upstream ` + udp + `
        }
    }
`

	err := loadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	doIntegrationTests(t, dnsTestCasesSRV, "test-1")
}
