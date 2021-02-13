package dnstap

import (
	"context"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/dnstap/msg"
	test "github.com/coredns/coredns/plugin/test"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

func testCase(t *testing.T, tapq, tapr *tap.Message, q, r *dns.Msg) {
	w := writer{t: t}
	w.queue = append(w.queue, tapq, tapr)
	h := Dnstap{
		Next: test.HandlerFunc(func(_ context.Context,
			w dns.ResponseWriter, _ *dns.Msg) (int, error) {

			return 0, w.WriteMsg(r)
		}),
		io: &w,
	}
	_, err := h.ServeDNS(context.TODO(), &test.ResponseWriter{}, q)
	if err != nil {
		t.Fatal(err)
	}
}

type writer struct {
	t     *testing.T
	queue []*tap.Message
}

func (w *writer) Dnstap(e tap.Dnstap) {
	if len(w.queue) == 0 {
		w.t.Error("Message not expected")
	}

	ex := w.queue[0]
	got := e.Message

	if string(ex.QueryAddress) != string(got.QueryAddress) {
		w.t.Errorf("Expected source address %s, got %s", ex.QueryAddress, got.QueryAddress)
	}
	if string(ex.ResponseAddress) != string(got.ResponseAddress) {
		w.t.Errorf("Expected response address %s, got %s", ex.ResponseAddress, got.ResponseAddress)
	}
	if *ex.QueryPort != *got.QueryPort {
		w.t.Errorf("Expected port %d, got %d", *ex.QueryPort, *got.QueryPort)
	}
	if *ex.SocketFamily != *got.SocketFamily {
		w.t.Errorf("Expected socket family %d, got %d", *ex.SocketFamily, *got.SocketFamily)
	}
	w.queue = w.queue[1:]
}

func TestDnstap(t *testing.T) {
	q := test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg()
	r := test.Case{
		Qname: "example.org.", Qtype: dns.TypeA,
		Answer: []dns.RR{
			test.A("example.org. 3600	IN	A 10.0.0.1"),
		},
	}.Msg()
	tapq := testMessage() // leave type unset for deepEqual
	msg.SetType(tapq, tap.Message_CLIENT_QUERY)
	tapr := testMessage()
	msg.SetType(tapr, tap.Message_CLIENT_RESPONSE)
	testCase(t, tapq, tapr, q, r)
}

func testMessage() *tap.Message {
	inet := tap.SocketFamily_INET
	udp := tap.SocketProtocol_UDP
	port := uint32(40212)
	return &tap.Message{
		SocketFamily:   &inet,
		SocketProtocol: &udp,
		QueryAddress:   net.ParseIP("10.240.0.1"),
		QueryPort:      &port,
	}
}
