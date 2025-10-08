package dnstest

import (
	"net"

	"github.com/coredns/coredns/plugin/pkg/reuseport"

	"github.com/miekg/dns"
)

// A Server is an DNS server listening on a system-chosen port on the local
// loopback interface, for use in end-to-end DNS tests.
type Server struct {
	Addr string // Address where the server listening.

	UDP *dns.Server
	TCP *dns.Server
}

// NewServer starts and returns a new Server. The caller should call Close when
// finished, to shut it down.
func NewServer(f dns.HandlerFunc) *Server {
	dns.HandleFunc(".", f)

	udpC := make(chan bool)
	tcpC := make(chan bool)

	udp := &dns.Server{}
	tcp := &dns.Server{}

	for range 5 { // 5 attempts
		tcp.Listener, _ = reuseport.Listen("tcp", ":0")
		if tcp.Listener == nil {
			continue
		}

		udp.PacketConn, _ = net.ListenPacket("udp", tcp.Listener.Addr().String())
		if udp.PacketConn != nil {
			break
		}

		// perhaps UPD port is in use, try again
		tcp.Listener.Close()
		tcp.Listener = nil
	}
	if tcp.Listener == nil {
		panic("dnstest.NewServer(): failed to create new server")
	}

	udp.NotifyStartedFunc = func() { close(udpC) }
	tcp.NotifyStartedFunc = func() { close(tcpC) }
	go udp.ActivateAndServe()
	go tcp.ActivateAndServe()

	<-udpC
	<-tcpC

	return &Server{UDP: udp, TCP: tcp, Addr: tcp.Listener.Addr().String()}
}

// NewMultipleServer starts and returns a new Server(multiple). The caller should call Close when
// finished, to shut it down.
func NewMultipleServer(f dns.HandlerFunc) *Server {
	udpC := make(chan bool)
	tcpC := make(chan bool)

	udp := &dns.Server{
		Handler: f,
	} // udp
	tcp := &dns.Server{
		Handler: f,
	} // tcp

	for range 5 { // 5 attempts
		tcp.Listener, _ = reuseport.Listen("tcp", ":0")
		if tcp.Listener == nil {
			continue
		}

		udp.PacketConn, _ = net.ListenPacket("udp", tcp.Listener.Addr().String())
		if udp.PacketConn != nil {
			break
		}

		// perhaps UPD port is in use, try again
		tcp.Listener.Close()
		tcp.Listener = nil
	}
	if tcp.Listener == nil {
		panic("dnstest.NewServer(): failed to create new server")
	}

	udp.NotifyStartedFunc = func() { close(udpC) }
	tcp.NotifyStartedFunc = func() { close(tcpC) }
	go udp.ActivateAndServe()
	go tcp.ActivateAndServe()

	<-udpC
	<-tcpC

	return &Server{UDP: udp, TCP: tcp, Addr: tcp.Listener.Addr().String()}
}

// Close shuts down the server.
func (s *Server) Close() {
	s.UDP.Shutdown()
	s.TCP.Shutdown()
}
