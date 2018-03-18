package doh

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/miekg/dns"
)

func TestRequest(t *testing.T) {
	const ex = "example.org."

	m := new(dns.Msg)
	m.SetQuestion(ex, dns.TypeDNSKEY)

	out, _ := m.Pack()
	req, err := http.NewRequest("POST", "https://"+ex+":443", bytes.NewReader(out))
	if err != nil {
		t.Errorf("Failure to make request: %s", err)
	}
	req.Header.Set("content-type", MimeType)
	req.Header.Set("accept", MimeType)

	m, err = RequestToMsg(req)
	if err != nil {
		t.Fatalf("Failure to get message from request: %s", err)
	}

	if x := m.Question[0].Name; x != ex {
		t.Errorf("qname expected %s, got %s", ex, x)
	}
	if x := m.Question[0].Qtype; x != dns.TypeDNSKEY {
		t.Errorf("qname expected %d, got %d", x, dns.TypeDNSKEY)
	}
}
