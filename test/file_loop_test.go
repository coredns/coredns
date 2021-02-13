package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const loopDB = `example.com. 500 IN SOA ns1.outside.com. root.example.com. 3 604800 86400 2419200 604800
example.com. 500 IN NS ns1.outside.com.
a.example.com. 500 IN CNAME b.example.com.
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

	m := new(dns.Msg)
	m.SetQuestion("something.foo.example.com.", dns.TypeA)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not exchange msg: %s", err)
	}

	// This should not loop, don't really care about the correctness of the answer.
	// Currently we return servfail in the file lookup.go file.
	// For now: document current behavior in this test.
	if r.Rcode != dns.RcodeServerFailure {
		t.Errorf("Rcode should be dns.RcodeServerFailure: %d", r.Rcode)

	}
}
