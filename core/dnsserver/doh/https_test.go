package doh

import (
	"testing"
)

func TestRequest(t *testing.T) {
	const ex = "example.org."

	m := new(Msg)
	m.SetQuestion(ex, TypeDNSKEY)

	req, err := MsgToRequest(m, "https://example.org:443")
	if err != nil {
		t.Errorf("failure to make request: %s", err)
	}

	m, err = RequestToMsg(req)
	if err != nil {
		t.Fatalf("failure to get message from request: %s", err)
	}

	if x := m.Question[0].Name; x != ex {
		t.Errorf("qname expected %s, got %s", ex, x)
	}
	if x := m.Question[0].Qtype; x != TypeDNSKEY {
		t.Errorf("qname expected %d, got %d", x, TypeDNSKEY)
	}
}
