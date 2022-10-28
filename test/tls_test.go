package test

import (
	"crypto/tls"
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

func TestTLS(t *testing.T) {
	tempCorefile := `%s {
        tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem
        whoami
    }`

	dot, doh := ":1053", ":8443"
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	answerLength := 0

	tests := []struct {
		server    string
		tlsConfig *tls.Config
	}{
		{fmt.Sprintf("tls://.%s", dot),
			&tls.Config{InsecureSkipVerify: true},
		},
		{fmt.Sprintf("tls://.%s", dot),
			&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"dot"}},
		},
		{fmt.Sprintf("tls://.%s https://.%s", dot, doh),
			&tls.Config{InsecureSkipVerify: true},
		},
		{fmt.Sprintf("tls://.%s https://.%s", dot, doh),
			&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"dot"}},
		},
	}

	for _, tc := range tests {
		ex, _, _, err := CoreDNSServerAndPorts(fmt.Sprintf(tempCorefile, tc.server))
		if err != nil {
			t.Fatalf("Could not get CoreDNS serving instance: %s", err)
		}

		client := dns.Client{
			Net:       "tcp-tls",
			TLSConfig: tc.tlsConfig,
		}
		r, _, err := client.Exchange(m, dot)

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
		ex.Stop()
	}
}
