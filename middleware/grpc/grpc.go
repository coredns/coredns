// Package grpc implements an HTTP handler that responds to grpc checks.
package grpc

import (
	"crypto/tls"
	"log"
	"net"

	context "golang.org/x/net/context"
	grpclib "google.golang.org/grpc"

	"github.com/miekg/dns"
	"github.com/miekg/coredns/core/dnsserver"
	"github.com/miekg/coredns/middleware/grpc/pb"
)

type grpc struct {
	addr   string
	tls    *tls.Config
	server *grpclib.Server
	config *dnsserver.Config
}

func (g *grpc) Startup() error {
	if g.addr == "" && g.tls == nil {
		g.addr = defHttp
	} else if g.addr == "" {
		g.addr = defHttps
	}

	var ln net.Listener
	var err error
	if g.tls == nil {
		ln, err = net.Listen("tcp", g.addr)
	} else {
		ln, err = tls.Listen("tcp", g.addr, g.tls)
	}
	if err != nil {
		log.Printf("[ERROR] Failed to start grpc handler: %s", err)
		return err
	}

	g.server = grpclib.NewServer()
	pb.RegisterDnsServiceServer(g.server, g)
	go func() {
		g.server.Serve(ln)
	}()
	return nil
}

func (g *grpc) Query(ctx context.Context, in *pb.DnsPacket) (*pb.DnsPacket, error) {
	msg := new(dns.Msg)
	err := msg.Unpack(in.Msg)
	if err != nil {
		return nil, err
	}

	//TODO(jbelamaric): this is junk
	a := &net.IPAddr{IP:net.ParseIP("127.0.0.1")}
	w := &response{localAddr: a, remoteAddr: a}
	g.config.Server.ServeDNS(w, msg)

	packed, err := w.Msg.Pack()
	if err != nil {
		return nil, err
	}
	return &pb.DnsPacket{Msg: packed}, nil
}

func (g *grpc) Shutdown() error {
	if g.server != nil {
		g.server.Stop()
	}
	return nil
}

type response struct {
	localAddr	net.Addr
	remoteAddr	net.Addr
	Msg		*dns.Msg
}

func (r *response) LocalAddr() net.Addr {
	return r.localAddr
}

func (r *response) RemoteAddr() net.Addr {
	return r.remoteAddr
}

func (r *response) WriteMsg(m *dns.Msg) error {
	r.Msg = m
	return nil
}

func (r *response) Write(b []byte) (int, error) {
	r.Msg = new(dns.Msg)
	return len(b), r.Msg.Unpack(b)
}

func (r *response) Close() error {
	return nil
}

func (r *response) TsigStatus() error {
	return nil
}

func (r *response) TsigTimersOnly(b bool) {
	return
}

func (r *response) Hijack() {
	return
}

const (
	defHttp  = ":80"
	defHttps = ":443"
)
