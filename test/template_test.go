package test

import (
	"testing"

	"github.com/miekg/dns"
)

func TestTemplateWithParseInt(t *testing.T) {
	corefile := `.:0 {
    template IN A example {
      match "^ip0a(?P<b>[a-f0-9]{2})(?P<c>[a-f0-9]{2})(?P<d>[a-f0-9]{2})[.]example[.]$"
      answer "{{ .Name }} 60 IN A 10.{{ parseInt .Group.b 16 8 }}.{{ parseInt .Group.c 16 8 }}.{{ parseInt .Group.d 16 8 }}"
    }
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("ip0a5f0c09.example.", dns.TypeA)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not send msg: %s", err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
	if len(r.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %v", len(r.Answer))
	}
	if x := r.Answer[0].(*dns.A).A.String(); x != "10.95.12.9" {
		t.Fatalf("Failed to get address for A record, expected 10.95.12.9 got %s", x)
	}
}
