package test

import (
	"os"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestSecondaryFallthrough(t *testing.T) {
	// Create zone file for primary - has www.example.org A 127.0.0.1
	primaryZone, rm1, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042745 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)

  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.

www    IN A    127.0.0.1
`)
	if err != nil {
		t.Fatalf("Failed to create primary zone: %s", err)
	}
	defer rm1()

	// Create zone file for fallback server - has other.example.org A 10.10.10.10
	fallbackZone, rm2, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042745 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)

  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.

other  IN A    10.10.10.10
`)
	if err != nil {
		t.Fatalf("Failed to create fallback zone: %s", err)
	}
	defer rm2()

	// Start primary server (serves zone via AXFR)
	primaryCorefile := `example.org:0 {
		file ` + primaryZone + `
		transfer {
			to *
		}
	}`
	primary, _, primaryTCP, err := CoreDNSServerAndPorts(primaryCorefile)
	if err != nil {
		t.Fatalf("Could not get primary CoreDNS instance: %s", err)
	}
	defer primary.Stop()

	// Start fallback server (answers queries forwarded by secondary)
	fallbackCorefile := `example.org:0 {
		file ` + fallbackZone + `
	}`
	fallback, fallbackUDP, _, err := CoreDNSServerAndPorts(fallbackCorefile)
	if err != nil {
		t.Fatalf("Could not get fallback CoreDNS instance: %s", err)
	}
	defer fallback.Stop()

	// Start secondary with fallthrough + forward to fallback
	secondaryCorefile := `example.org:0 {
		secondary {
			transfer from ` + primaryTCP + `
			fallthrough
		}
		forward . ` + fallbackUDP + `
	}`
	sec, secUDP, _, err := CoreDNSServerAndPorts(secondaryCorefile)
	if err != nil {
		t.Fatalf("Could not get secondary CoreDNS instance: %s", err)
	}
	defer sec.Stop()

	// Wait for zone transfer to complete
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)
	var r *dns.Msg
	for range 10 {
		r, _ = dns.Exchange(m, secUDP)
		if r != nil && len(r.Answer) != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if r == nil || len(r.Answer) == 0 {
		t.Fatal("Zone transfer did not complete")
	}

	// Test 1: www.example.org exists in secondary zone - should return answer from zone
	m = new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)
	r, err = dns.Exchange(m, secUDP)
	if err != nil {
		t.Fatalf("Expected to receive reply for www.example.org, but got error: %s", err)
	}
	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR for www.example.org, got %s", dns.RcodeToString[r.Rcode])
	}
	if len(r.Answer) != 1 {
		t.Fatalf("Expected 1 answer for www.example.org, got %d", len(r.Answer))
	}
	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record for www.example.org, got %T", r.Answer[0])
	}
	if a.A.String() != "127.0.0.1" {
		t.Fatalf("Expected www.example.org to be 127.0.0.1, got %s", a.A.String())
	}

	// Test 2: other.example.org does NOT exist in secondary zone
	// With fallthrough, query should pass to forward plugin which queries fallback server
	m = new(dns.Msg)
	m.SetQuestion("other.example.org.", dns.TypeA)
	r, err = dns.Exchange(m, secUDP)
	if err != nil {
		t.Fatalf("Expected to receive reply for other.example.org, but got error: %s", err)
	}
	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR for fallthrough query other.example.org, got %s", dns.RcodeToString[r.Rcode])
	}
	if len(r.Answer) != 1 {
		t.Fatalf("Expected 1 answer from fallback for other.example.org, got %d", len(r.Answer))
	}
	a, ok = r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record from fallback for other.example.org, got %T", r.Answer[0])
	}
	if a.A.String() != "10.10.10.10" {
		t.Fatalf("Expected fallback answer 10.10.10.10, got %s", a.A.String())
	}
}

func TestEmptySecondaryZone(t *testing.T) {
	// Corefile that fails to transfer example.org.
	corefile := `example.org:0 {
		secondary {
			transfer from 127.0.0.1:1717
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
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected reply to be a SERVFAIL, got %d", resp.Rcode)
	}
}

func TestSecondaryZoneTransfer(t *testing.T) {
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + ` {
		}
		transfer {
			to *
		}
	}`

	i, _, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	corefile = `example.org:0 {
		secondary {
			transfer from ` + tcp + `
		}
	}`

	i1, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i1.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)

	var r *dns.Msg
	// This is now async; we need to wait for it to be transferred.
	for range 10 {
		r, _ = dns.Exchange(m, udp)
		if len(r.Answer) != 0 {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}
	if len(r.Answer) == 0 {
		t.Fatalf("Expected answer section")
	}
}

func TestIxfrResponse(t *testing.T) {
	// ixfr query with current soa should return single packet with that soa (no transfer needed).
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + ` {
		}
		transfer {
			to *
		}
	}`

	i, _, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeIXFR)
	m.Ns = []dns.RR{test.SOA("example.org. IN SOA sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600")} // copied from exampleOrg

	var r *dns.Msg
	c := new(dns.Client)
	c.Net = "tcp"
	// This is now async; we need to wait for it to be transferred.
	for range 10 {
		r, _, _ = c.Exchange(m, tcp)
		if len(r.Answer) != 0 {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("Expected answer section with single RR")
	}
	soa, ok := r.Answer[0].(*dns.SOA)
	if !ok {
		t.Fatalf("Expected answer section with SOA RR")
	}
	if soa.Serial != 2015082541 {
		t.Fatalf("Serial should be %d, got %d", 2015082541, soa.Serial)
	}
}

func TestRetryInitialTransfer(t *testing.T) {
	// Start up a secondary that expects to transfer from a master that doesn't exist yet
	corefile := `example.org:0 {
		secondary {
			transfer from 127.0.0.1:5399
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
	// Expect that the query will fail
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected reply to be a SERVFAIL, got %d", resp.Rcode)
	}

	// Now spin up the master server
	name, rm, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042745 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)

  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.

www    IN A    127.0.0.1
www    IN AAAA ::1
`)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefileMaster := `example.org:5399 {
		file ` + name + `
		transfer {
          to *
        }
	}`

	master, _, _, err := CoreDNSServerAndPorts(corefileMaster)
	if err != nil {
		t.Fatalf("Could not start CoreDNS master: %s", err)
	}
	defer master.Stop()

	retry := time.Tick(time.Millisecond * 100)
	timeout := time.Tick(time.Second * 5)

	for {
		select {
		case <-retry:
			m = new(dns.Msg)
			m.SetQuestion("www.example.org.", dns.TypeA)
			resp, err = dns.Exchange(m, udp)
			if err != nil {
				continue
			}
			// Expect the query to succeed
			if resp.Rcode != dns.RcodeSuccess {
				continue
			}
			return
		case <-timeout:
			t.Fatal("Timed out trying for successful response.")
			return
		}
	}
}

func TestSecondaryZoneNotify(t *testing.T) {
	// Now spin up the master server
	name, rm, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042745 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)

  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.
`)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefileMaster := `example.org:53553 {
    bind 127.0.0.1
	file ` + name + ` {
		reload 0.01s
	}
	transfer {
		to 127.0.0.1:53554
	}
}`

	master, _, _, err := CoreDNSServerAndPorts(corefileMaster)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer master.Stop()

	corefileSecondary := `example.org:53554 {
		bind 127.0.0.1
		secondary {
			transfer from 127.0.0.1:53553 
		}
		transfer {
			to 127.0.0.1:53555
		}
	}`
	secondary, _, _, err := CoreDNSServerAndPorts(corefileSecondary)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer secondary.Stop()

	corefile := `example.org:53555 {
        bind 127.0.0.1
		secondary {
			transfer from 127.0.0.1:53554
		}
	}`

	svr, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer svr.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeSOA)

	var r *dns.Msg
	// This is now async; we need to wait for it to be transferred.
	for range 10 {
		r, _ = dns.Exchange(m, udp)
		if len(r.Answer) != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(r.Answer) == 0 {
		t.Fatalf("Expected answer section")
	}

	m = new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)
	r, _ = dns.Exchange(m, udp)
	if len(r.Answer) != 0 {
		t.Fatalf("Expected no answer section, got %d answers", len(r.Answer))
	}

	os.WriteFile(name, []byte(`$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042746 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)

  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.
www    IN A    127.0.0.1
`), 0644)

	// This is now async; we need to wait for it to be transferred.
	for range 10 {
		r, _ = dns.Exchange(m, udp)
		if len(r.Answer) != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("Expected one RR in answer section got %d", len(r.Answer))
	}
}
