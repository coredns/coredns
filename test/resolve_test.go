package test

import (
	"testing"

	"github.com/miekg/dns"

	_ "github.com/coredns/coredns/plugin/resolve"
	_ "github.com/coredns/coredns/plugin/template"
)

func TestResolveA(t *testing.T) {
	corefile := `.:0 {
        resolve .

 		# CNAME
		template IN ANY cname.test. {
			match ".*"
			answer "cname.test. 60 IN CNAME target.test."
		}

		# Target
		template IN ANY target.test. {
			match ".*"
			answer "target.test 60 IN A 1.2.3.4"
		}
}
`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Test that an A query returns a CNAME and an A record
	testname := "A Query:"
	m := new(dns.Msg)
	m.SetQuestion("cname.test.", dns.TypeA)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("%s Could not send msg: %s", testname, err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("%s Rcode should not be dns.RcodeServerFailure", testname)
	}
	if len(r.Answer) != 2 {
		t.Fatalf("%s Expected 2 answers, got %v", testname, len(r.Answer))
	}
	if x := r.Answer[0].(*dns.CNAME).Target; x != "target.test." {
		t.Fatalf("%s Expected CNAME target to be target.test. got %s", testname, x)
	}
	if x := r.Answer[1].(*dns.A).A.String(); x != "1.2.3.4" {
		t.Fatalf("%s Incorrect A record for CNAME, expected 1.2.3.4 got %s", testname, x)
	}
}

func TestResolveCNAME(t *testing.T) {
	corefile := `.:0 {
        resolve .

 		# CNAME
		template IN ANY cname.test. {
			match ".*"
			answer "cname.test. 60 IN CNAME target.test."
		}

		# Target
		template IN A target.test. {
			match ".*"
			answer "target.test 60 IN A 1.2.3.4"
		}
}
`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Test that a CNAME query only returns a CNAME
	testname := "CNAME Query:"
	m := new(dns.Msg)
	m.SetQuestion("cname.test.", dns.TypeCNAME)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("%s could not send msg: %s", testname, err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("%s rcode should not be dns.RcodeServerFailure", testname)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("%s expected 1 answer, got %v", testname, len(r.Answer))
	}
	if x := r.Answer[0].(*dns.CNAME).Target; x != "target.test." {
		t.Fatalf("%s expected CNAME target to be target.test. got %s", testname, x)
	}
}

func TestResolveACNAMELoop(t *testing.T) {
	corefile := `.:0 {
        resolve .

 		# CNAME
		template IN ANY cname.test. {
			match ".*"
			answer "cname.test. 60 IN CNAME target.test."
		}

		# Target
		template IN ANY target.test. {
			match ".*"
			answer "target.test. 60 IN CNAME cname.test."
		}
}
`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Test that an A query returns a CNAME and an A record
	m := new(dns.Msg)
	m.SetQuestion("cname.test.", dns.TypeA)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not send msg: %s", err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
	if len(r.Answer) != 2 {
		t.Fatalf("Expected 2 answers, got %v", len(r.Answer))
	}
	if x := r.Answer[0].(*dns.CNAME).Target; x != "target.test." {
		t.Fatalf("Expected first CNAME target to be target.test. got %s", x)
	}
	if x := r.Answer[1].(*dns.CNAME).Target; x != "cname.test." {
		t.Fatalf("Expected second CNAME target to be cname.test. got %s", x)
	}
}

func TestResolveSRV(t *testing.T) {
	corefile := `.:0 {
        resolve .

 		# CNAME
		template IN ANY cname.test. {
			match ".*"
			answer "cname.test. 60 IN CNAME srv.test."
		}

		# SRV
		template IN SRV srv.test. {
			match ".*"
			answer "_http._tcp.srv.test. 3600 IN SRV 0 0 80  ep.test."
		}

		# A
		template IN A ep.test. {
			match ".*"
			answer "ep.test. 60 IN A 1.2.3.4"
		}

		# AAAA
		template IN AAAA ep.test. {
			match ".*"
			answer "ep.test. 60 IN AAAA ::1:2:3:4"
		}
}
`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Test that an A query returns a CNAME and an A record
	m := new(dns.Msg)
	m.SetQuestion("cname.test.", dns.TypeSRV)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not send msg: %s", err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
	if len(r.Answer) != 2 {
		t.Fatalf("Expected 2 answers, got %v", len(r.Answer))
	}
	if x := r.Answer[0].(*dns.CNAME).Target; x != "srv.test." {
		t.Fatalf("Expected CNAME target to be srv.test. got %s", x)
	}
	if x := r.Answer[1].(*dns.SRV).Target; x != "ep.test." {
		t.Fatalf("Expected SRV target to be ep.test. got %s", x)
	}
	if len(r.Extra) != 2 {
		t.Fatalf("Expected 1 extra record, got %v", len(r.Answer))
	}
	if x := r.Extra[0].(*dns.A).A.String(); x != "1.2.3.4" {
		t.Fatalf("Incorrect extra record for SRV, expected 1.2.3.4 got %s", x)
	}
	if x := r.Extra[1].(*dns.AAAA).AAAA.String(); x != "::1:2:3:4" {
		t.Fatalf("Incorrect extra record for SRV, expected ::1:2:3:4 got %s", x)
	}

}
