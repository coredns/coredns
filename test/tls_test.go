package test

import (
	"crypto/tls"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSoverTLS(t *testing.T) {
	corefile := `tls://.:1053 {
        tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem
        whoami
    }`
	qname := "example.com."
	qtype := dns.TypeA
	answerLength := 0

	ex, _, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer ex.Stop()

	m := new(dns.Msg)
	m.SetQuestion(qname, qtype)
	client := dns.Client{
		Net:       "tcp-tls",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	r, _, err := client.Exchange(m, tcp)

	if err != nil {
		t.Fatalf("Could not exchange msg: %s", err)
	}

	if n := len(r.Answer); n != answerLength {
		t.Fatalf("Expected %v answers, got %v", answerLength, n)
	}
	if n := len(r.Extra); n != 2 {
		t.Errorf("Expected 2 RRs in additional section, but got %d", n)
	}
	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", r.Rcode)
	}
}
