package transfer

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestNotify(t *testing.T) {
	// Set up a local udp listener to listen for udp notify messages
	pc, err := net.ListenPacket("udp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	defer pc.Close()

	// set up and start the notify channel listener
	transfer := Transfer{
		xfrs: []*xfr{{
			Zones: []string{"example.com."},
			to:    hosts{addr: &notifyOpts{source: &net.UDPAddr{IP: net.ParseIP("127.0.0.1")}}}, // send the dns notifies to our udp listener
		}},
	}
	stop := make(chan struct{})
	data := make(chan []string)
	go transfer.Notify(data, stop)
	defer func() { stop <- struct{}{} }()

	// send message to notify data channel
	data <- []string{"example.com."}

	// read from the udp listener, timing out quickly
	buffer := make([]byte, 1024)
	pc.SetDeadline(time.Now().Add(time.Second))
	_, clientAddr, err := pc.ReadFrom(buffer)
	if err != nil {
		t.Fatal(err)
	}

	// Unpack and inspect the message, ensuring it is a notify message for the expected zone
	notifyMsg := dns.Msg{}
	err = notifyMsg.Unpack(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if notifyMsg.Opcode != dns.OpcodeNotify {
		t.Fatalf("Expected opcode Notify(4), got %v ", notifyMsg.Opcode)
	}
	if len(notifyMsg.Question) != 1 {
		t.Fatalf("Expected one question, got %v ", len(notifyMsg.Question))
	}
	if notifyMsg.Question[0].Name != "example.com." {
		t.Fatalf("Expected zone example.com., got %v ", notifyMsg.Question[0])
	}

	// Write a positive response back to the client
	msg := dns.Msg{MsgHdr: dns.MsgHdr{Id: notifyMsg.Id, Response: true, Opcode: dns.OpcodeNotify}}
	buf := make([]byte, 1024)
	p, _ := msg.PackBuffer(buf)
	if err != nil {
		t.Error(err)
	}
	_, err = pc.WriteTo(p, clientAddr)

	if err != nil {
		t.Error(err)
	}
}
