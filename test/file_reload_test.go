package test

import (
	"os"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestZoneReload(t *testing.T) {
	name, rm, err := test.TempFile(".", relativeReloadZone)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	// Both stanzas load the same relative zone file with their own origin.
	corefile := `
	example.org:0 {
		file ` + name + ` {
			reload 0.01s
		}
	}
	example.net:0 {
		file ` + name + `
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
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}
	if len(resp.Answer) != 2 {
		t.Fatalf("Expected two RR in answer section got %d", len(resp.Answer))
	}

	// Remove RR from the Apex
	if err := os.WriteFile(name, []byte(relativeReloadZoneUpdated), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond) // reload time, with some race insurance

	resp, err = dns.Exchange(m, udp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected one RR in answer section got %d", len(resp.Answer))
	}
}

const relativeReloadZone = `
@ IN SOA sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600
@ IN NS b.iana-servers.net.
@ IN NS a.iana-servers.net.
@ IN A 127.0.0.1
@ IN A 127.0.0.2
`

const relativeReloadZoneUpdated = `
@ IN SOA sns.dns.icann.org. noc.dns.icann.org. 2016082541 7200 3600 1209600 3600
@ IN NS b.iana-servers.net.
@ IN NS a.iana-servers.net.
@ IN A 127.0.0.2
`
