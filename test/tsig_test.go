package test

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

var tsigKey = "tsig.key."
var tsigSecret = "i9M+00yrECfVZG2qCjr4mPpaGim/Bq+IWMiNrLjUO4Y="

var corefile = `.:0 {
		tsig {
    		secret ` + tsigKey + ` ` + tsigSecret + `
		}
		hosts {
			1.2.3.4 test
		}
	}`

func TestTsig(t *testing.T) {
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("test.", dns.TypeA)
	m.SetTsig(tsigKey, dns.HmacSHA256, 300, time.Now().Unix())

	client := dns.Client{Net: "udp", TsigSecret: map[string]string{tsigKey: tsigSecret}}
	r, _, err := client.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not send msg: %s", err)
	}
	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Rcode should be dns.RcodeSuccess")
	}
	tsig := r.IsTsig()
	if tsig == nil {
		t.Fatalf("Respose was not TSIG")
	}
	if tsig.Error != dns.RcodeSuccess {
		t.Fatalf("TSIG Error code should be dns.RcodeSuccess")
	}
}

func TestTsigBadKey(t *testing.T) {
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("test.", dns.TypeA)
	m.SetTsig("bad.key.", dns.HmacSHA256, 300, time.Now().Unix())

	client := dns.Client{Net: "udp", TsigSecret: map[string]string{"bad.key.": tsigSecret}}
	r, _, err := client.Exchange(m, udp)

	if err != dns.ErrAuth {
		t.Fatalf("Expected \"dns: bad authentication\" error, got: %s", err)
	}
	if r.Rcode != dns.RcodeNotAuth {
		t.Fatalf("Rcode should be dns.RcodeNotAuth")
	}
	tsig := r.IsTsig()
	if tsig == nil {
		t.Fatalf("Respose was not TSIG")
	}
	if tsig.Error != dns.RcodeBadKey {
		t.Fatalf("TSIG Error code should be dns.RcodeBadKey")
	}
	if tsig.MAC != "" {
		t.Fatalf("TSIG MAC should be empty")
	}
	if tsig.MACSize != 0 {
		t.Fatalf("TSIG MACSize should be 0")
	}
	if tsig.TimeSigned != 0 {
		t.Fatalf("TSIG TimeSigned should be 0")
	}
}

// TODO: TestTsigBadSig

// TODO: TestTsigBadTime
