package dnsutil

import (
	"testing"

	"github.com/miekg/dns"
)

func TestUnpackRequest(t *testing.T) {
	request := new(dns.Msg)
	request.SetQuestion("example.org.", dns.TypeA)

	wire, err := request.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackRequest(wire); err != nil {
		t.Fatalf("UnpackRequest() rejected a valid request: %v", err)
	}

	request.Question = append(request.Question, request.Question[0])
	wire, err = request.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackRequest(wire); err == nil {
		t.Fatal("UnpackRequest() accepted multiple questions")
	}
}

func TestUnpackRequestWithAcceptFuncNilFallsBackToDefault(t *testing.T) {
	request := new(dns.Msg)
	request.SetUpdate("example.org.")
	wire, err := request.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackRequestWithAcceptFunc(wire, nil); err == nil {
		t.Fatal("expected a nil accept func to reject an UPDATE exactly like the default policy does")
	}
}

func TestUnpackRequestWithAcceptFuncHonorsCustomPolicy(t *testing.T) {
	request := new(dns.Msg)
	request.SetUpdate("example.org.")
	wire, err := request.Pack()
	if err != nil {
		t.Fatal(err)
	}

	allowUpdate := func(h dns.Header) dns.MsgAcceptAction {
		opcode := int(h.Bits>>11) & 0xF
		if opcode == dns.OpcodeUpdate && h.Qdcount == 1 {
			return dns.MsgAccept
		}
		return dns.DefaultMsgAcceptFunc(h)
	}
	msg, err := UnpackRequestWithAcceptFunc(wire, allowUpdate)
	if err != nil {
		t.Fatalf("expected a custom accept func to allow UPDATE, got: %v", err)
	}
	if msg.Opcode != dns.OpcodeUpdate {
		t.Fatalf("expected the unpacked message to keep its UPDATE opcode, got %d", msg.Opcode)
	}
}
