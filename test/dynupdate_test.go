package test

import (
	"fmt"
	"testing"
	"time"

	plugintest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const (
	dynUpdateKey    = "update-key.example.org."
	dynUpdateSecret = "i9M+00yrECfVZG2qCjr4mPpaGim/Bq+IWMiNrLjUO4Y="
)

const dynUpdateZone = `$ORIGIN example.org.
@ 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60
@ 60 IN NS ns.example.org.
ns 60 IN A 192.0.2.53
www 60 IN A 192.0.2.1
`

func TestDynUpdateUDPAndTCP(t *testing.T) {
	zoneFile, remove, err := plugintest.TempFile(".", dynUpdateZone)
	if err != nil {
		t.Fatalf("creating zone file: %v", err)
	}
	defer remove()

	corefile := fmt.Sprintf(`example.org:0 {
		tsig {
			secret %s %s
			require_opcode UPDATE
		}
		dynupdate {
			file %s
			allow %s * TXT
		}
	}`, dynUpdateKey, dynUpdateSecret, zoneFile, dynUpdateKey)
	server, udp, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("starting CoreDNS: %v", err)
	}
	defer server.Stop()

	for _, tc := range []struct {
		name string
		net  string
		addr string
	}{
		{name: "udp", net: "udp", addr: udp},
		{name: "tcp", net: "tcp", addr: tcp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := tc.name + ".example.org."
			rr, err := dns.NewRR(owner + ` 60 IN TXT "dynamic"`)
			if err != nil {
				t.Fatalf("creating update RR: %v", err)
			}

			msg := new(dns.Msg).SetUpdate("example.org.")
			msg.Insert([]dns.RR{rr})
			msg.SetTsig(dynUpdateKey, dns.HmacSHA256, 300, time.Now().Unix())
			client := &dns.Client{Net: tc.net, TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret}}
			resp, _, err := client.Exchange(msg, tc.addr)
			if err != nil {
				t.Fatalf("sending %s UPDATE: %v", tc.name, err)
			}
			if resp.Rcode != dns.RcodeSuccess {
				t.Fatalf("UPDATE rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
			}
			if resp.Opcode != dns.OpcodeUpdate {
				t.Fatalf("response opcode = %d, want UPDATE", resp.Opcode)
			}

			query := new(dns.Msg)
			query.SetQuestion(owner, dns.TypeTXT)
			answer, _, err := (&dns.Client{Net: "udp"}).Exchange(query, udp)
			if err != nil {
				t.Fatalf("querying updated record: %v", err)
			}
			if answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
				t.Fatalf("updated query response = rcode %s, %d answers", dns.RcodeToString[answer.Rcode], len(answer.Answer))
			}
			if got := answer.Answer[0].String(); got != owner+"\t60\tIN\tTXT\t\"dynamic\"" {
				t.Fatalf("updated record = %q", got)
			}
		})
	}
}

func TestDynUpdateRejectsUnsignedRequest(t *testing.T) {
	zoneFile, remove, err := plugintest.TempFile(".", dynUpdateZone)
	if err != nil {
		t.Fatalf("creating zone file: %v", err)
	}
	defer remove()

	corefile := fmt.Sprintf(`example.org:0 {
		tsig {
			secret %s %s
			require_opcode UPDATE
		}
		dynupdate {
			file %s
			allow %s * TXT
		}
	}`, dynUpdateKey, dynUpdateSecret, zoneFile, dynUpdateKey)
	server, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("starting CoreDNS: %v", err)
	}
	defer server.Stop()

	msg := new(dns.Msg).SetUpdate("example.org.")
	rr, err := dns.NewRR("unsigned.example.org. 60 IN TXT \"denied\"")
	if err != nil {
		t.Fatalf("creating update RR: %v", err)
	}
	msg.Insert([]dns.RR{rr})
	resp, err := dns.Exchange(msg, udp)
	if err != nil {
		t.Fatalf("sending unsigned UPDATE: %v", err)
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("unsigned UPDATE rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
}

func TestDynUpdateIsNotServedFromCache(t *testing.T) {
	zoneFile, remove, err := plugintest.TempFile(".", dynUpdateZone)
	if err != nil {
		t.Fatalf("creating zone file: %v", err)
	}
	defer remove()

	corefile := fmt.Sprintf(`example.org:0 {
		tsig {
			secret %s %s
			require_opcode UPDATE
		}
		cache
		dynupdate {
			file %s
			allow %s * TXT
		}
	}`, dynUpdateKey, dynUpdateSecret, zoneFile, dynUpdateKey)
	server, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("starting CoreDNS: %v", err)
	}
	defer server.Stop()

	owner := "example.org."
	query := new(dns.Msg)
	query.SetQuestion(owner, dns.TypeSOA)
	if resp, _, err := (&dns.Client{Net: "udp"}).Exchange(query, udp); err != nil {
		t.Fatalf("priming SOA cache: %v", err)
	} else if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("priming query response = %s with %d answers, want SOA", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}

	rr, err := dns.NewRR("cached.example.org. 60 IN TXT \"dynamic\"")
	if err != nil {
		t.Fatalf("creating update RR: %v", err)
	}
	update := new(dns.Msg).SetUpdate("example.org.")
	update.Insert([]dns.RR{rr})
	update.SetTsig(dynUpdateKey, dns.HmacSHA256, 300, time.Now().Unix())
	client := &dns.Client{Net: "udp", TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret}}
	resp, _, err := client.Exchange(update, udp)
	if err != nil {
		t.Fatalf("sending UPDATE after cached query: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("UPDATE rcode after cached query = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	// Use a distinct cache key to verify the committed record without being
	// masked by the SOA entry deliberately primed above.
	query.SetQuestion("cached.example.org.", dns.TypeTXT)
	query.CheckingDisabled = true
	resp, _, err = (&dns.Client{Net: "udp"}).Exchange(query, udp)
	if err != nil {
		t.Fatalf("querying updated record: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 || resp.Answer[0].String() != "cached.example.org.\t60\tIN\tTXT\t\"dynamic\"" {
		t.Fatalf("updated response = %#v, want one TXT answer", resp)
	}
}

func TestDynUpdateAXFRIncludesUpdatedRecord(t *testing.T) {
	zoneFile, remove, err := plugintest.TempFile(".", dynUpdateZone)
	if err != nil {
		t.Fatalf("creating zone file: %v", err)
	}
	defer remove()

	corefile := fmt.Sprintf(`example.org:0 {
		tsig {
			secret %s %s
			require_opcode UPDATE
		}
		transfer {
			to *
		}
		dynupdate {
			file %s
			allow %s * TXT
		}
	}`, dynUpdateKey, dynUpdateSecret, zoneFile, dynUpdateKey)
	server, _, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("starting CoreDNS: %v", err)
	}
	defer server.Stop()

	rr, err := dns.NewRR("axfr.example.org. 60 IN TXT \"dynamic\"")
	if err != nil {
		t.Fatalf("creating update RR: %v", err)
	}
	update := new(dns.Msg).SetUpdate("example.org.")
	update.Insert([]dns.RR{rr})
	update.SetTsig(dynUpdateKey, dns.HmacSHA256, 300, time.Now().Unix())
	client := &dns.Client{
		Net:        "tcp",
		TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret},
	}
	resp, _, err := client.Exchange(update, tcp)
	if err != nil {
		t.Fatalf("sending signed UPDATE: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("UPDATE rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}

	transfer := &dns.Transfer{
		DialTimeout: 5 * time.Second,
		ReadTimeout: 5 * time.Second,
	}
	query := new(dns.Msg)
	query.SetAxfr("example.org.")
	envelopes, err := transfer.In(query, tcp)
	if err != nil {
		t.Fatalf("starting AXFR: %v", err)
	}
	var records []dns.RR
	for envelope := range envelopes {
		if envelope.Error != nil {
			t.Fatalf("AXFR envelope: %v", envelope.Error)
		}
		records = append(records, envelope.RR...)
	}
	if len(records) < 2 || records[0].Header().Rrtype != dns.TypeSOA || records[len(records)-1].Header().Rrtype != dns.TypeSOA {
		t.Fatalf("unexpected AXFR framing: %v", records)
	}
	found := false
	for _, transferred := range records {
		if transferred.String() == rr.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("AXFR did not include %s", rr)
	}
}
