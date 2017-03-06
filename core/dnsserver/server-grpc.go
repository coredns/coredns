package dnsserver

import (
	"fmt"
	"net"

	"github.com/miekg/dns"
)

// GRPCServer represents an instance of a DNS-over-gRPC.
type GRPCServer struct {
	*Server
}

// NewGRPCServer returns a new CoreDNS GRPC server and compiles all middleware in to it.
func NewGRPCServer(addr string, group []*Config) (*GRPCServer, error) {

	s, err := NewServer(addr, group)
	if err != nil {
		return nil, err
	}

	// Add read decorators to s[tcp]

	return &GRPCServer{Server: s}, nil
}

// Serve implements caddy.TCPServer interface.
func (s *GRPCServer) Serve(l net.Listener) error {
	s.m.Lock()

	// Only fill out the TCP server for this one.
	s.server[tcp] = &dns.Server{Listener: l, Net: "tcp-tls", Handler: s.mux}
	s.m.Unlock()

	return s.server[tcp].ActivateAndServe()
}

// ServePacket This implements caddy.UDPServer interface.
func (s *GRPCServer) ServePacket(p net.PacketConn) error { return nil }

// Listen implements caddy.TCPServer interface.
func (s *GRPCServer) Listen() (net.Listener, error) {

	// Remove, but show our 'tls' directive has been picked up.
	for _, conf := range s.zones {
		fmt.Printf("%q\n", conf.TLSConfig)
	}

	l, err := net.Listen("tcp", s.Addr[len(TransportGRPC+"://"):])
	if err != nil {
		return nil, err
	}
	return l, nil
}

// ListenPacket implements caddy.UDPServer interface.
func (s *GRPCServer) ListenPacket() (net.PacketConn, error) { return nil, nil }

// OnStartupComplete lists the sites served by this server
// and any relevant information, assuming Quiet is false.
func (s *GRPCServer) OnStartupComplete() {
	if Quiet {
		return
	}

	for zone, config := range s.zones {
		fmt.Println(TransportGRPC + "://" + zone + ":" + config.Port)
	}
}
