package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// TestShed checks that with the shed plugin installed a query is answered
// over both UDP (through the plugin's deferred single-writer path) and
// TCP (which shed passes through), and that its drop counter is exported.
func TestShed(t *testing.T) {
	corefile := `.:0 {
		shed
		prometheus localhost:0
		whoami
	}`

	i, udp, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)

	if r, err := dns.Exchange(m, udp); err != nil || r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected UDP reply, got %v: %v", r, err)
	}
	c := &dns.Client{Net: "tcp"}
	if r, _, err := c.Exchange(m, tcp); err != nil || r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected TCP reply, got %v: %v", r, err)
	}

	data := test.Scrape("http://" + metrics.ListenAddr + "/metrics")
	got, labels := test.MetricValue("coredns_shed_dropped_total", data)
	if got != "0" {
		t.Errorf("Expected coredns_shed_dropped_total 0, but got %s", got)
	}
	if labels["reason"] == "" {
		t.Errorf("Expected coredns_shed_dropped_total to carry a reason label, got %v", labels)
	}
}
