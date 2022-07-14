package test

import (
	"crypto/tls"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSoverTLS(t *testing.T) {
	testcases := []struct {
		name         string
		config       string
		Qname        string
		Qtype        uint16
		Answer       []dns.RR
		AnswerLength int
	}{
		{
			name: "Test whoami with DNS over TLS",
			config: `tls://.:1053 {
                tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem
                whoami
            }`,
			Qname:        "example.com.",
			Qtype:        dns.TypeA,
			Answer:       []dns.RR{},
			AnswerLength: 0,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ex, _, tcp, err := CoreDNSServerAndPorts(tc.config)
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			defer ex.Stop()

			m := new(dns.Msg)
			m.SetQuestion(tc.Qname, tc.Qtype)
			client := dns.Client{
				Net:       "tcp-tls",
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
			}
			r, _, err := client.Exchange(m, tcp)

			if err != nil {
				t.Fatalf("Could not exchange msg: %s", err)
			}

			if n := len(r.Answer); n != tc.AnswerLength {
				t.Fatalf("Expected %v answers, got %v", tc.AnswerLength, n)
			}
			if n := len(r.Extra); n != 2 {
				t.Errorf("Expected 2 RRs in additional section, but got %d", n)
			}

		})
	}
}
