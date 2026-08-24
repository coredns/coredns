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
