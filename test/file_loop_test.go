package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const loopDB = `example.com. 500 IN SOA ns1.outside.com. root.example.com. 3 604800 86400 2419200 604800
example.com. 500 IN NS ns1.outside.com.
a.example.com. 500 IN CNAME b.example.com.
alias.example.com. 500 IN DNAME alias.example.com.
redirect.example.com. 500 IN DNAME target.example.com.
www.target.example.com. 500 IN A 192.0.2.1
*.foo.example.com. 500 IN CNAME bar.foo.example.com.`

func TestFileLoop(t *testing.T) {
	name, rm, err := test.TempFile(".", loopDB)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	// Corefile with for example without proxy section.
	corefile := `example.com:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	tests := []struct {
		name            string
		qname           string
		wantRcode       int
		checkAnswer     bool
		wantAnswerTypes []uint16
	}{
		{"wildcard CNAME", "something.foo.example.com.", dns.RcodeServerFailure, false, nil},
		{"self-referential DNAME", "www.alias.example.com.", dns.RcodeServerFailure, true, nil},
		{"non-looping DNAME", "www.redirect.example.com.", dns.RcodeSuccess, true, []uint16{dns.TypeDNAME, dns.TypeCNAME, dns.TypeA}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, dns.TypeA)

			r, err := dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Could not exchange msg: %s", err)
			}

			if r.Rcode != tc.wantRcode {
				t.Errorf("Rcode should be %d, got %d", tc.wantRcode, r.Rcode)
			}
			if !tc.checkAnswer {
				return
			}
			if len(r.Answer) != len(tc.wantAnswerTypes) {
				t.Fatalf("Expected %d answer records, got %d", len(tc.wantAnswerTypes), len(r.Answer))
			}
			for i, qtype := range tc.wantAnswerTypes {
				if r.Answer[i].Header().Rrtype != qtype {
					t.Errorf("Answer %d should have type %s, got %s", i, dns.TypeToString[qtype], dns.TypeToString[r.Answer[i].Header().Rrtype])
				}
			}
			if len(tc.wantAnswerTypes) == 0 && (len(r.Ns) != 0 || len(r.Extra) != 0) {
				t.Errorf("Response sections should be empty, got %d answer, %d authority, and %d additional records", len(r.Answer), len(r.Ns), len(r.Extra))
			}
		})
	}
}
