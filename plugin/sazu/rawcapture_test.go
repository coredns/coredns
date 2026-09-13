package sazu

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func TestRawCapturePutTakeRoundTrip(t *testing.T) {
	c := NewRawCapture(time.Second, 16)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242}
	raw := []byte{0x12, 0x34, 0xAA, 0xBB, 0xCC}

	c.Put(addr, raw)

	got, ok := c.Take(addr, 0x1234)
	if !ok {
		t.Fatalf("expected a captured entry to be found")
	}
	if string(got) != string(raw) {
		t.Fatalf("got %x, want %x", got, raw)
	}
}

func TestRawCaptureTakeIsSingleUse(t *testing.T) {
	c := NewRawCapture(time.Second, 16)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242}
	c.Put(addr, []byte{0x00, 0x01, 0xFF})

	if _, ok := c.Take(addr, 1); !ok {
		t.Fatalf("expected first Take to find the entry")
	}
	if _, ok := c.Take(addr, 1); ok {
		t.Fatalf("expected second Take of the same entry to find nothing")
	}
}

func TestRawCaptureTakeWithoutPutFindsNothing(t *testing.T) {
	c := NewRawCapture(time.Second, 16)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242}
	if _, ok := c.Take(addr, 1); ok {
		t.Fatalf("expected no entry for an address/id that was never Put")
	}
}

func TestRawCaptureDifferentAddressesDoNotCollide(t *testing.T) {
	c := NewRawCapture(time.Second, 16)
	a1 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}
	a2 := &net.UDPAddr{IP: net.ParseIP("127.0.0.2"), Port: 1}
	c.Put(a1, []byte{0x00, 0x05, 0x01})
	c.Put(a2, []byte{0x00, 0x05, 0x02})

	got1, ok := c.Take(a1, 5)
	if !ok || got1[2] != 0x01 {
		t.Fatalf("expected a1's own entry, got %v ok=%v", got1, ok)
	}
	got2, ok := c.Take(a2, 5)
	if !ok || got2[2] != 0x02 {
		t.Fatalf("expected a2's own entry, got %v ok=%v", got2, ok)
	}
}

func TestRawCaptureExpiredEntryNotReturned(t *testing.T) {
	c := NewRawCapture(10*time.Millisecond, 16)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242}
	c.Put(addr, []byte{0x00, 0x01, 0xFF})

	time.Sleep(50 * time.Millisecond)

	if _, ok := c.Take(addr, 1); ok {
		t.Fatalf("expected an expired entry to not be returned")
	}
}

func TestRawCaptureEvictsOldestWhenFull(t *testing.T) {
	c := NewRawCapture(time.Minute, 2)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242}
	c.Put(addr, []byte{0x00, 0x01, 0x01}) // id 1, oldest
	c.Put(addr, []byte{0x00, 0x02, 0x02}) // id 2
	c.Put(addr, []byte{0x00, 0x03, 0x03}) // id 3, should evict id 1

	if _, ok := c.Take(addr, 1); ok {
		t.Fatalf("expected the oldest entry (id 1) to have been evicted")
	}
	if _, ok := c.Take(addr, 2); !ok {
		t.Fatalf("expected id 2 to still be present")
	}
	if _, ok := c.Take(addr, 3); !ok {
		t.Fatalf("expected id 3 to still be present")
	}
}

// captureAssertHandler is a minimal plugin.Handler that, for every
// request, retrieves the raw bytes RawCapture stashed for it (correlated
// by the response writer's remote address and the message ID -- exactly
// how a real SAZU handler would) and records whether that lookup found
// the exact bytes sent.
type captureAssertHandler struct {
	capture *RawCapture
	found   atomic.Bool
	matched atomic.Bool
}

func (h *captureAssertHandler) Name() string { return "capture-assert" }

func (h *captureAssertHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	raw, ok := h.capture.Take(w.RemoteAddr(), r.Id)
	h.found.Store(ok)
	if ok {
		var decoded dns.Msg
		if err := decoded.Unpack(raw); err == nil &&
			len(decoded.Question) == 1 && decoded.Question[0].Name == r.Question[0].Name {
			h.matched.Store(true)
		}
	}
	m := new(dns.Msg)
	m.SetReply(r)
	_ = w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// TestRawCaptureEndToEndThroughUDPDecorateReaderFunc proves the full path
// this package relies on: RawCapture.DecorateReaderFunc installed as
// core/dnsserver.Config.UDPDecorateReaderFunc actually delivers, inside a
// real plugin's ServeDNS call, the literal bytes a client sent over a
// real UDP round trip through CoreDNS's own listener -- no second
// listener involved.
func TestRawCaptureEndToEndThroughUDPDecorateReaderFunc(t *testing.T) {
	capture := NewRawCapture(5*time.Second, 64)
	handler := &captureAssertHandler{capture: capture}

	cfg := &dnsserver.Config{
		Zone:        "example.com.",
		Transport:   "dns",
		ListenHosts: []string{"127.0.0.1"},
		Port:        "0",
	}
	cfg.AddPlugin(func(plugin.Handler) plugin.Handler { return handler })
	cfg.UDPDecorateReaderFunc = capture.DecorateReaderFunc

	s, err := dnsserver.NewServer("127.0.0.1:0", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket failed: %v", err)
	}
	defer pc.Close()

	go func() { _ = s.ServePacket(pc) }()
	defer s.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	if _, err := dns.Exchange(m, pc.LocalAddr().String()); err != nil {
		t.Fatalf("dns.Exchange failed: %v", err)
	}

	if !handler.found.Load() {
		t.Fatalf("expected the handler to find a captured entry for its own request")
	}
	if !handler.matched.Load() {
		t.Fatalf("expected the captured bytes to decode to the same question that was sent")
	}
}

// TestRawCaptureEndToEndThroughTCPDecorateReaderFunc mirrors the UDP
// version above for core/dnsserver.Config.TCPDecorateReaderFunc -- the
// transport sazuctl actually uses for anything of meaningful size (see
// push.go), since a real signed push routinely exceeds the path MTU and
// gets silently dropped as an IP fragment over UDP on real networks.
func TestRawCaptureEndToEndThroughTCPDecorateReaderFunc(t *testing.T) {
	capture := NewRawCapture(5*time.Second, 64)
	handler := &captureAssertHandler{capture: capture}

	cfg := &dnsserver.Config{
		Zone:        "example.com.",
		Transport:   "dns",
		ListenHosts: []string{"127.0.0.1"},
		Port:        "0",
	}
	cfg.AddPlugin(func(plugin.Handler) plugin.Handler { return handler })
	cfg.TCPDecorateReaderFunc = capture.DecorateReaderFunc

	s, err := dnsserver.NewServer("127.0.0.1:0", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer l.Close()

	go func() { _ = s.Serve(l) }()
	defer s.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	co, err := dns.DialTimeout("tcp", l.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("DialTimeout failed: %v", err)
	}
	defer co.Close()
	if err := co.WriteMsg(m); err != nil {
		t.Fatalf("WriteMsg failed: %v", err)
	}
	if _, err := co.ReadMsg(); err != nil {
		t.Fatalf("ReadMsg failed: %v", err)
	}

	if !handler.found.Load() {
		t.Fatalf("expected the handler to find a captured entry for its own request")
	}
	if !handler.matched.Load() {
		t.Fatalf("expected the captured bytes to decode to the same question that was sent")
	}
}
