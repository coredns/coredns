// +build k8s

package test

import (
	"log"
	"testing"
	"time"

	"github.com/mholt/caddy"
	"github.com/miekg/dns"
)

func makeRR(s string) dns.RR {
	rr, err := dns.NewRR(s)
	if err != nil {
		log.Printf("Bad test RR (%s): %s", s, err)
	}
	return rr
}

type testquery struct {
	fqdn  string
	qtype uint16
}

type testanswer struct {
	rcode int
	rrs   []dns.RR
}

type testdefn struct {
	query  testquery
	answer testanswer
}

// Test data
// TODO: Fix the actual RR values
var testdata = []testdefn{
	{testquery{"mynginx.demo.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"bogusservice.demo.svc.coredns.local.", dns.TypeA},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"mynginx.*.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"mynginx.any.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"bogusservice.*.svc.coredns.local.", dns.TypeA},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"bogusservice.any.svc.coredns.local.", dns.TypeA},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.demo.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"any.demo.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"any.test.svc.coredns.local.", dns.TypeA},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.test.svc.coredns.local.", dns.TypeA},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.*.svc.coredns.local.", dns.TypeA},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"mynginx.demo.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"bogusservice.demo.svc.coredns.local.", dns.TypeSRV},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"mynginx.*.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"mynginx.any.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"bogusservice.*.svc.coredns.local.", dns.TypeSRV},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"bogusservice.any.svc.coredns.local.", dns.TypeSRV},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.demo.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"any.demo.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
	{testquery{"any.test.svc.coredns.local.", dns.TypeSRV},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.test.svc.coredns.local.", dns.TypeSRV},
		// TODO: fix NameError bug so this can be used instead of success: testanswer{dns.RcodeNameError,
		testanswer{dns.RcodeSuccess,
			nil,
		},
	},
	{testquery{"*.*.svc.coredns.local.", dns.TypeSRV},
		testanswer{dns.RcodeSuccess,
			[]dns.RR{makeRR("mynginx.demo.svc.coredns.local. A 10.3.0.10"),
				makeRR("webserver.demo.svc.coredns.local. A 10.3.0.10")},
		},
	},
}

func createTestServer(t *testing.T, corefile string) (*caddy.Instance, string) {
	server, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	udp, _ := CoreDNSServerPorts(server, 0)
	if udp == "" {
		t.Fatalf("Could not get UDP listening port")
	}

	return server, udp
}

func TestKubernetesIntegration(t *testing.T) {
	corefile :=
		`.:0 {
    kubernetes coredns.local {
                endpoint http://localhost:8080
		namespaces demo
    }

`
	server, udp := createTestServer(t, corefile)
	defer server.Stop()

	// Work-around for timing condition that results in no-data being returned in
	// test environment.
	time.Sleep(5 * time.Second)

	for _, td := range testdata {
		dnsClient := new(dns.Client)
		dnsMessage := new(dns.Msg)

		dnsMessage.SetQuestion(td.query.fqdn, td.query.qtype)
		//		dnsMessage.SetEdns0(4096, true)

		res, _, err := dnsClient.Exchange(dnsMessage, udp)
		if err != nil {
			t.Fatalf("Could not send query: %s", err)
		}

		// check the answer
		if res.Rcode != td.answer.rcode {
			t.Errorf("Expected rcode %d but got %d for query %v", td.answer.rcode, res.Rcode, td.query)
		}

		if len(res.Answer) != len(td.answer.rrs) {
			t.Errorf("Expected %d answers but got %d for query %v", len(td.answer.rrs), len(res.Answer), td.query)
		}

		//TODO: Check the actual RR values
	}
}
