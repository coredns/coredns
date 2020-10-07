package test

import (
	"testing"

	"github.com/miekg/dns"
)

func TestNoPlugins(t *testing.T) {
	corefile := `example.org:0 {
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %v", err)
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("Expected rcode to be %d, got %d", dns.RcodeRefused, resp.Rcode)
	}
}
