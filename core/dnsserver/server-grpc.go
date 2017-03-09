package dnsserver

import (
	"errors"
	"fmt"
	"net"

	// TODO(miek): can't have these cross refs, pb needs to move here, or in the top-level dir
	"github.com/coredns/coredns/middleware/proxy/pb"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// GRPCServer represents an instance of a DNS-over-gRPC.
type GRPCServer struct {
	*Server
	grpcServer *grpc.Server
	listenAddr net.Addr
}

// NewGRPCServer returns a new CoreDNS GRPC server and compiles all middleware in to it.
func NewGRPCServer(addr string, group []*Config) (*GRPCServer, error) {

	s, err := NewServer(addr, group)
	if err != nil {
		return nil, err
	}

	return &GRPCServer{Server: s}, nil
}

// Serve implements caddy.TCPServer interface.
func (s *GRPCServer) Serve(l net.Listener) error {
	s.m.Lock()

	// Only fill out the gRPC server for this one.
	s.grpcServer = grpc.NewServer()

	// trace foo... TODO(miek)

	pb.RegisterDnsServiceServer(s.grpcServer, s)

	s.listenAddr = l.Addr()

	s.m.Unlock()

	return s.grpcServer.Serve(l)
}

// ServePacket implements caddy.UDPServer interface.
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

func (s *GRPCServer) Stop() (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.grpcServer != nil {
		s.grpcServer.Stop()
	}
	return
}

// Query is the main entry-point into the gRPC server. From here we call ServeDNS like
// any normal server. We use a custom responseWriter to pick up the bytes we need to write
// back to the client as a protobuf.
func (s *GRPCServer) Query(ctx context.Context, in *pb.DnsPacket) (*pb.DnsPacket, error) {
	msg := new(dns.Msg)
	err := msg.Unpack(in.Msg)
	if err != nil {
		return nil, err
	}

	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("no peer in gRPC context")
	}

	a, ok := p.Addr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("no TCP peer in gRPC context: %v", p.Addr)
	}

	r := &net.IPAddr{IP: a.IP}
	w := &gRPCresponse{localAddr: s.listenAddr, remoteAddr: r}

	s.ServeDNS(ctx, w, msg)

	packed, err := w.Msg.Pack()
	if err != nil {
		return nil, err
	}

	return &pb.DnsPacket{Msg: packed}, nil
}

func (g *GRPCServer) Shutdown() error {
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
	return nil
}

type gRPCresponse struct {
	localAddr  net.Addr
	remoteAddr net.Addr
	Msg        *dns.Msg
}

// Write is the hack that makes this work. It does not actually write the message
// but returns the bytes we need to to write in r. We can then pick this up in Query
// and write a proper protobuf back to the client.
func (r *gRPCresponse) Write(b []byte) (int, error) {
	r.Msg = new(dns.Msg)
	return len(b), r.Msg.Unpack(b)
}

// These methods implement the dns.ResponseWriter interface from Go DNS.
func (r *gRPCresponse) Close() error              { return nil }
func (r *gRPCresponse) TsigStatus() error         { return nil }
func (r *gRPCresponse) TsigTimersOnly(b bool)     { return }
func (r *gRPCresponse) Hijack()                   { return }
func (r *gRPCresponse) LocalAddr() net.Addr       { return r.localAddr }
func (r *gRPCresponse) RemoteAddr() net.Addr      { return r.remoteAddr }
func (r *gRPCresponse) WriteMsg(m *dns.Msg) error { r.Msg = m; return nil }
