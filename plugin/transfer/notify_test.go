package transfer

import (
	"net"
	"testing"
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
