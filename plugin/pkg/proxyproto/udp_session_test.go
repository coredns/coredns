package proxyproto

import (
	"net"
	"testing"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

func udpAddr(host string, port int) *net.UDPAddr {
	return &net.UDPAddr{IP: net.ParseIP(host), Port: port}
}

// testHeader builds a minimal PPv2 header with the given source address.
func testHeader(src *net.UDPAddr) *proxyproto.Header {
	return &proxyproto.Header{
		Version:    2,
		SourceAddr: src,
	}
}

func TestSessionKey(t *testing.T) {
	addr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 5000}
	got := sessionKey(addr)
	want := "udp://10.0.0.1:5000"
	if got != want {
		t.Fatalf("sessionKey = %q, want %q", got, want)
	}
}

func newTestPacketConn(ttl time.Duration, maxSessions int) *PacketConn {
	return &PacketConn{
		UDPSessionTrackingTTL:         ttl,
		UDPSessionTrackingMaxSessions: maxSessions,
	}
}

func TestStoreAndLookupSession(t *testing.T) {
	pc := newTestPacketConn(time.Second, 0)
	remote := udpAddr("10.0.0.1", 5000)
	src := udpAddr("192.168.1.1", 12345)

	pc.storeSession(remote, testHeader(src))

	got, ok := pc.lookupSession(remote)
	if !ok {
		t.Fatal("expected session to be found")
	}
	if got.SourceAddr.String() != src.String() {
		t.Fatalf("expected SourceAddr %s, got %s", src, got.SourceAddr)
	}
}

func TestLookupSessionMiss(t *testing.T) {
	pc := newTestPacketConn(time.Second, 0)
	_, ok := pc.lookupSession(udpAddr("10.0.0.1", 5000))
	if ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestLookupSessionExpired(t *testing.T) {
	pc := newTestPacketConn(50*time.Millisecond, 0)
	remote := udpAddr("10.0.0.1", 5000)
	src := udpAddr("192.168.1.1", 12345)

	pc.storeSession(remote, testHeader(src))
	time.Sleep(100 * time.Millisecond)

	_, ok := pc.lookupSession(remote)
	if ok {
		t.Fatal("expected expired entry to be missing")
	}
}

func TestLookupSessionRefreshesTTL(t *testing.T) {
	ttl := 50 * time.Millisecond
	pc := newTestPacketConn(ttl, 0)
	remote := udpAddr("10.0.0.1", 5000)
	src := udpAddr("192.168.1.1", 12345)

	pc.storeSession(remote, testHeader(src))

	// Wait past half the TTL, then look up (which should refresh).
	time.Sleep(30 * time.Millisecond)
	_, ok := pc.lookupSession(remote)
	if !ok {
		t.Fatal("expected session to be found before TTL")
	}

	// Wait another 30ms. Original TTL would have expired (60ms > 50ms),
	// but the refresh from lookupSession should keep it alive.
	time.Sleep(30 * time.Millisecond)
	_, ok = pc.lookupSession(remote)
	if !ok {
		t.Fatal("expected session to survive after TTL refresh")
	}
}

func TestStoreSessionCustomMaxSessions(t *testing.T) {
	pc := newTestPacketConn(time.Second, 5)

	// Fill beyond custom cap.
	for i := range 10 {
		pc.storeSession(udpAddr("10.0.0.1", i), testHeader(udpAddr("1.1.1.1", i)))
	}

	if pc.sessionCache.Len() != 5 {
		t.Fatalf("expected cache capped at 5, got %d", pc.sessionCache.Len())
	}
}

func TestStoreSessionEvictsOldest(t *testing.T) {
	pc := newTestPacketConn(time.Minute, 2)
	r1 := udpAddr("10.0.0.1", 1)
	r2 := udpAddr("10.0.0.2", 2)
	r3 := udpAddr("10.0.0.3", 3)

	pc.storeSession(r1, testHeader(udpAddr("1.1.1.1", 1)))
	pc.storeSession(r2, testHeader(udpAddr("2.2.2.2", 2)))
	// Cache is full (cap=2). Storing r3 evicts r1.
	pc.storeSession(r3, testHeader(udpAddr("3.3.3.3", 3)))

	if _, ok := pc.lookupSession(r1); ok {
		t.Fatal("expected r1 to be evicted")
	}
	if _, ok := pc.lookupSession(r2); !ok {
		t.Fatal("expected r2 to be present")
	}
	if _, ok := pc.lookupSession(r3); !ok {
		t.Fatal("expected r3 to be present")
	}
}
