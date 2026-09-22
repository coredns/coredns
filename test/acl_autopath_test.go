package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
)

func TestACLAuthorizesAutopathExpandedName(t *testing.T) {
	resolvConf := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(
		resolvConf,
		[]byte("search public.example protected.example\n"),
		0644,
	); err != nil {
		t.Fatalf("Could not write resolv.conf: %s", err)
	}

	corefile := `.:0 {
		acl protected.example. {
			block
		}
		autopath ` + resolvConf + `
		hosts {
			192.0.2.123 secret.protected.example
		}
	}`

	server, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer server.Stop()

	tests := []struct {
		name  string
		qname string
	}{
		{
			name:  "direct protected query",
			qname: "secret.protected.example.",
		},
		{
			name:  "autopath expanded protected query",
			qname: "secret.public.example.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, dns.TypeA)

			r, err := dns.Exchange(m, udp)
			if err != nil {
				t.Fatalf("Query %q failed: %s", tc.qname, err)
			}
			if r.Rcode != dns.RcodeRefused {
				t.Fatalf(
					"Query %q: got rcode %s, want REFUSED; answers: %v",
					tc.qname,
					dns.RcodeToString[r.Rcode],
					r.Answer,
				)
			}
		})
	}
}
