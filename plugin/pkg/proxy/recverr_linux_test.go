//go:build linux

package proxy

import (
	"net"
	"syscall"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/transport"

	"golang.org/x/sys/unix"
)

func sockopt(t *testing.T, c syscall.Conn, level, opt int) int {
	t.Helper()
	rc, err := c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var v int
	var serr error
	if err := rc.Control(func(fd uintptr) { v, serr = unix.GetsockoptInt(int(fd), level, opt) }); err != nil {
		t.Fatal(err)
	}
	if serr != nil {
		t.Fatal(serr)
	}
	return v
}

// TestRecvErrUpstreamSockets checks that the UDP sockets for upstream queries and
// health checks get IP_RECVERR, and TCP sockets do not.
func TestRecvErrUpstreamSockets(t *testing.T) {
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()

	tr := newTransport("test_recverr", udp.LocalAddr().String())
	tr.Start()
	defer tr.Stop()
	pc, _, err := tr.Dial("udp")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.c.Close()
	if v := sockopt(t, pc.c.Conn.(*net.UDPConn), unix.SOL_IP, unix.IP_RECVERR); v != 1 {
		t.Errorf("upstream UDP socket: IP_RECVERR = %d, want 1", v)
	}

	hc := NewHealthChecker("test_recverr", transport.DNS, true, ".").(*dnsHc)
	hconn, err := hc.c.Dial(udp.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer hconn.Close()
	if v := sockopt(t, hconn.Conn.(*net.UDPConn), unix.SOL_IP, unix.IP_RECVERR); v != 1 {
		t.Errorf("health check UDP socket: IP_RECVERR = %d, want 1", v)
	}

	// source_address goes through setDialer, which must keep the option.
	hc.SetLocalAddress(net.ParseIP("127.0.0.1"))
	lconn, err := hc.c.Dial(udp.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer lconn.Close()
	if v := sockopt(t, lconn.Conn.(*net.UDPConn), unix.SOL_IP, unix.IP_RECVERR); v != 1 {
		t.Errorf("health check UDP socket with source address: IP_RECVERR = %d, want 1", v)
	}

	ttr := newTransport("test_recverr_tcp", tcp.Addr().String())
	ttr.Start()
	defer ttr.Stop()
	tpc, _, err := ttr.Dial("tcp")
	if err != nil {
		t.Fatal(err)
	}
	defer tpc.c.Close()
	if v := sockopt(t, tpc.c.Conn.(*net.TCPConn), unix.SOL_IP, unix.IP_RECVERR); v != 0 {
		t.Errorf("upstream TCP socket: IP_RECVERR = %d, want 0", v)
	}
}

// TestRecvErrUDP6 checks that IPv6 upstream sockets get IPV6_RECVERR.
func TestRecvErrUDP6(t *testing.T) {
	udp, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback:", err)
	}
	defer udp.Close()
	tr := newTransport("test_recverr6", udp.LocalAddr().String())
	tr.Start()
	defer tr.Stop()
	pc, _, err := tr.Dial("udp")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.c.Close()
	if v := sockopt(t, pc.c.Conn.(*net.UDPConn), unix.SOL_IPV6, unix.IPV6_RECVERR); v != 1 {
		t.Errorf("upstream UDP6 socket: IPV6_RECVERR = %d, want 1", v)
	}
}
