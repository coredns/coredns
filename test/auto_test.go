package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestAuto(t *testing.T) {
	t.Parallel()
	tmpdir := t.TempDir()

	corefile := `org:0 {
		auto {
			directory ` + tmpdir + ` db\.(.*) {1}
			reload 0.01s
		}
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("Expected reply to be REFUSED, got %d", resp.Rcode)
	}

	// Write db.example.org to get example.org.
	if err = os.WriteFile(filepath.Join(tmpdir, "db.example.org"), []byte(zoneContent), 0644); err != nil {
		t.Fatal(err)
	}

	for start := time.Now(); time.Since(start) < 5*time.Second; {
		resp, err = dns.Exchange(m, udp)
		if err != nil {
			t.Fatal("Expected to receive reply, but didn't")
		}
		if len(resp.Answer) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 RR in the answer section, got %d", len(resp.Answer))
	}

	// Remove db.example.org again.
	os.Remove(filepath.Join(tmpdir, "db.example.org"))

	for start := time.Now(); time.Since(start) < 5*time.Second; {
		resp, err = dns.Exchange(m, udp)
		if err != nil {
			t.Fatal("Expected to receive reply, but didn't")
		}
		if resp.Rcode == dns.RcodeRefused {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("Expected reply to be REFUSED, got %d", resp.Rcode)
	}
}

func TestAutoMixedCaseOrigin(t *testing.T) {
	t.Parallel()

	for _, filename := range []string{"db.example.org", "db.Example.ORG"} {
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, filename), []byte(zoneContent), 0644); err != nil {
				t.Fatal(err)
			}
			corefile := `org:0 {
				auto {
					directory ` + dir + `
					reload 0
				}
			}`
			i, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			defer i.Stop()

			for _, network := range []string{"udp", "tcp"} {
				addr := udp
				if network == "tcp" {
					addr = tcp
				}
				client := &dns.Client{Net: network}
				for _, tc := range []struct {
					name  string
					qtype uint16
					count int
				}{
					{"example.org.", dns.TypeSOA, 1},
					{"example.org.", dns.TypeNS, 2},
					{"www.example.org.", dns.TypeA, 1},
					{"WWW.Example.ORG.", dns.TypeA, 1},
				} {
					m := new(dns.Msg)
					m.SetQuestion(tc.name, tc.qtype)
					resp, _, err := client.Exchange(m, addr)
					if err != nil {
						t.Fatal(err)
					}
					if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative || len(resp.Answer) != tc.count {
						t.Errorf("%s %s %s: expected authoritative NOERROR with %d answers, got %s",
							network, tc.name, dns.TypeToString[tc.qtype], tc.count, resp)
					}
				}
			}
		})
	}
}

func TestAutoNonExistentZone(t *testing.T) {
	t.Parallel()
	tmpdir := t.TempDir()

	corefile := `.:0 {
		auto {
			directory ` + tmpdir + ` (.*) {1}
			reload 0.01s
		}
		errors stdout
	}`

	i, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	udp, _ := CoreDNSServerPorts(i, 0)
	if udp == "" {
		t.Fatal("Could not get UDP listening port")
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("Expected reply to be REFUSED, got %d", resp.Rcode)
	}
}

func TestAutoAXFR(t *testing.T) {
	t.Parallel()

	tmpdir := t.TempDir()

	corefile := `org:0 {
		auto {
			directory ` + tmpdir + ` db\.(.*) {1}
			reload 0.01s
		}
		transfer {
			to *
		}
	}`

	i, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	_, tcp := CoreDNSServerPorts(i, 0)
	if tcp == "" {
		t.Fatal("Could not get TCP listening port")
	}
	defer i.Stop()

	// Write db.example.org to get example.org.
	if err = os.WriteFile(filepath.Join(tmpdir, "db.example.org"), []byte(zoneContent), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // wait for it to be picked up

	tr := new(dns.Transfer)
	m := new(dns.Msg)
	m.SetAxfr("example.org.")
	c, err := tr.In(m, tcp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	l := 0
	for e := range c {
		l += len(e.RR)
	}

	if l != 5 {
		t.Fatalf("Expected response with %d RRs, got %d", 5, l)
	}
}

const zoneContent = `; testzone
@   IN SOA sns.dns.icann.org. noc.dns.icann.org. 2016082534 7200 3600 1209600 3600
    IN NS  a.iana-servers.net.
    IN NS  b.iana-servers.net.

www IN A   127.0.0.1
`
