package test

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/proxy"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestFallbackLookup(t *testing.T) {
	corefile := `fallback.org:0 {
                       fallback fallback.org {
                         on SERVFAIL 127.0.0.1:999
                      }	
                    }`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	var b bytes.Buffer
	log.SetOutput(&b)

	p := proxy.NewLookup([]string{udp})
	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}

	resp, err := p.Lookup(state, "nop.fallback.org.", dns.TypeA)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	// expect no answer section
	if len(resp.Answer) != 0 {
		t.Fatalf("Expected no RR in the answer section, got %d", len(resp.Answer))
	}
	if !strings.Contains(b.String(), `[INFO] Send fallback "nop.fallback.org." to "127.0.0.1:999"`) {
		t.Fatal("Expected to receive log but didn't")
	}
}
