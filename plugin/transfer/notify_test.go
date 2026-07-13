package transfer

import (
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestNotifyClientSource(t *testing.T) {
	c := notifyClient(&xfr{})
	if c.Dialer != nil {
		t.Fatalf("expected no dialer without a source address, got %#v", c.Dialer)
	}

	source := net.ParseIP("2001:db8::53")
	c = notifyClient(&xfr{source: source})
	if c.Dialer == nil {
		t.Fatal("expected dialer for source address")
	}

	addr, ok := c.Dialer.LocalAddr.(*net.UDPAddr)
	if !ok {
		t.Fatalf("expected UDP local address, got %T", c.Dialer.LocalAddr)
	}
	if !addr.IP.Equal(source) {
		t.Fatalf("expected source address %s, got %s", source, addr.IP)
	}
}

func TestNotifyMultipleFailures(t *testing.T) {
	// Start two local UDP listeners returning different rcodes
	l1, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l1.Close()

	l2, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l2.Close()

	serve := func(conn net.PacketConn, rcode int) {
		buf := make([]byte, 512)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			msg := new(dns.Msg)
			if err := msg.Unpack(buf[:n]); err != nil {
				continue
			}
			msg.SetReply(msg)
			msg.Rcode = rcode
			out, _ := msg.Pack()
			conn.WriteTo(out, addr)
		}
	}

	go serve(l1, dns.RcodeServerFailure)
	go serve(l2, dns.RcodeRefused)

	tr := &Transfer{
		xfrs: []*xfr{
			{
				Zones: []string{"example.com."},
				to:    []string{l1.LocalAddr().String(), l2.LocalAddr().String()},
			},
		},
	}

	err = tr.Notify("example.com.")
	if err == nil {
		t.Fatal("expected error from Notify, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, l1.LocalAddr().String()) || !strings.Contains(errStr, "SERVFAIL") {
		t.Errorf("expected error to contain %q and SERVFAIL, got: %v", l1.LocalAddr().String(), errStr)
	}
	if !strings.Contains(errStr, l2.LocalAddr().String()) || !strings.Contains(errStr, "REFUSED") {
		t.Errorf("expected error to contain %q and REFUSED, got: %v", l2.LocalAddr().String(), errStr)
	}
}
