package test

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestObtainCertOnStartup(t *testing.T) {
	certmagicDataPath := "/tmp/certmagic"
	pebbleTestConfig := "test/config/pebble-config.json"
	pebbleStrictMode := false
	resolverAddress := "127.0.0.1:1053"

	testcases := []struct {
		name            string
		config          string
		resolverAddress string
		Qname           string
		Qtype           uint16
		Answer          []dns.RR
		AnswerLength    int
		WhoAmI          bool
		ExpectedIP      string
	}{
		{
			name: "Test ACME whoami",
			config: `tls://.:1053 {
                tls acme {
                    domain example.com
                    ca     localhost:14963/dir
                    certpath /tmp/certmagic/
                    cacert test/certs/pebble.minica.pem
                    port 1053
                }
                whoami
            }`,
			Qname:        "example.com.",
			Qtype:        dns.TypeTXT,
			Answer:       []dns.RR{},
			AnswerLength: 0,
			WhoAmI:       true,
			ExpectedIP:   "",
		},
		{
			name: "Test ACME forward to Google",
			config: `tls://.:1053 {
                tls acme {
                    domain example.com
                    ca     localhost:14963/dir
                    certpath /tmp/certmagic/
                    cacert test/certs/pebble.minica.pem
                    port 1053
                }
                forward . 8.8.8.8
            }`,
			Qname:        "example.com.",
			Qtype:        dns.TypeA,
			Answer:       []dns.RR{},
			AnswerLength: 1,
			WhoAmI:       false,
			ExpectedIP:   "93.184.216.34",
		},
	}
	go func() {
		pebbleServer(resolverAddress, pebbleTestConfig, pebbleStrictMode)
	}()
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.RemoveAll(certmagicDataPath); err != nil {
				t.Logf("Nothing to remove in %q", certmagicDataPath)
			}

			ex, _, tcp, err := CoreDNSServerAndPorts(tc.config)
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			defer ex.Stop()

			m := new(dns.Msg)
			m.SetQuestion(tc.Qname, tc.Qtype)
			client := dns.Client{
				Net:          "tcp-tls",
				TLSConfig:    &tls.Config{InsecureSkipVerify: true},
				Timeout:      5 * time.Second,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 5 * time.Second,
			}
			r, _, err := client.Exchange(m, tcp)

			if err != nil {
				t.Fatalf("Could not exchange msg: %s", err)
			}

			if n := len(r.Answer); n != tc.AnswerLength {
				t.Fatalf("Expected %v answers, got %v", tc.AnswerLength, n)
			}
			if tc.AnswerLength > 0 {
				if r.Answer[0].(*dns.A).A.String() != tc.ExpectedIP {
					t.Errorf("Expected %s for example.com, got: %s", tc.ExpectedIP, r.Answer[0].(*dns.A).A.String())
				}
			}
			if tc.WhoAmI {
				if n := len(r.Extra); n != 2 {
					t.Errorf("Expected 2 RRs in additional section, but got %d", n)
				}
			}
		})
	}
}
