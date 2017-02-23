package proxy

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"

	"github.com/miekg/coredns/middleware/proxy/pb"
	"github.com/miekg/coredns/middleware/trace"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type grpcClient struct {
	dialOpts []grpc.DialOption
	clients  map[string]pb.DnsServiceClient
	conns    []*grpc.ClientConn
	upstream *staticUpstream
}

func newGrpcClient(tls *tls.Config, u *staticUpstream) *grpcClient {
	g := &grpcClient{upstream: u}

	if tls == nil {
		g.dialOpts = append(g.dialOpts, grpc.WithInsecure())
	} else {
		g.dialOpts = append(g.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tls)))
	}
	g.clients = map[string]pb.DnsServiceClient{}

	return g
}

func (g *grpcClient) Exchange(ctx context.Context, addr string, state request.Request) (*dns.Msg, error) {
	msg, err := state.Req.Pack()
	if err != nil {
		return nil, err
	}

	reply, err := g.clients[addr].Query(ctx, &pb.DnsPacket{Msg: msg})
	if err != nil {
		return nil, err
	}
	d := new(dns.Msg)
	err = d.Unpack(reply.Msg)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (g *grpcClient) Protocol() string { return "grpc" }

func (g *grpcClient) OnShutdown(p *Proxy) error {
	for i, conn := range g.conns {
		err := conn.Close()
		if err != nil {
			log.Printf("[WARNING] Error closing connection %d: %s\n", i, err)
		}
	}
	return nil
}

func (g *grpcClient) OnStartup(p *Proxy) error {
	dialOpts := g.dialOpts
	if p.Trace != nil {
		if t, ok := p.Trace.(trace.Trace); ok {
			dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(t.Tracer())))
		} else {
			log.Printf("[WARNING] Wrong type for trace middleware reference: %s", p.Trace)
		}
	}
	for _, host := range g.upstream.Hosts {
		conn, err := grpc.Dial(host.Name, dialOpts...)
		if err != nil {
			log.Printf("[WARNING] Skipping gRPC host '%s' due to Dial error: %s\n", host.Name, err)
		} else {
			g.clients[host.Name] = pb.NewDnsServiceClient(conn)
			g.conns = append(g.conns, conn)
		}
	}
	return nil
}
