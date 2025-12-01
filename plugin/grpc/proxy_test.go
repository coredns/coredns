package grpc

import (
	"context"
	"errors"
	"net"
	"path"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestProxy(t *testing.T) {
	tests := map[string]struct {
		p       *Proxy
		res     *dns.Msg
		wantErr bool
	}{
		"response_ok": {
			p:       &Proxy{},
			res:     &dns.Msg{},
			wantErr: false,
		},
		"nil_response": {
			p:       &Proxy{},
			res:     nil,
			wantErr: true,
		},
		"tls": {
			p:       &Proxy{dialOpts: []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(nil))}},
			res:     &dns.Msg{},
			wantErr: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var mock *testServiceClient
			if tt.res != nil {
				msg, err := tt.res.Pack()
				if err != nil {
					t.Fatalf("Error packing response: %s", err.Error())
				}
				mock = &testServiceClient{&pb.DnsPacket{Msg: msg}, nil}
			} else {
				mock = &testServiceClient{nil, errors.New("server error")}
			}
			tt.p.client = mock

			_, err := tt.p.query(context.TODO(), new(dns.Msg))
			if err != nil && !tt.wantErr {
				t.Fatalf("Error query(): %s", err.Error())
			}
		})
	}
}

type testServiceClient struct {
	dnsPacket *pb.DnsPacket
	err       error
}

func (m testServiceClient) Query(ctx context.Context, in *pb.DnsPacket, opts ...grpc.CallOption) (*pb.DnsPacket, error) {
	return m.dnsPacket, m.err
}

func TestProxyUnix(t *testing.T) {
	tdir := t.TempDir()

	fd := path.Join(tdir, "test.grpc")
	listener, err := net.Listen("unix", fd)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	defer listener.Close()

	server := grpc.NewServer()
	pb.RegisterDnsServiceServer(server, &grpcDnsServiceServer{})

	go server.Serve(listener)
	defer server.Stop()

	c := caddy.NewTestController("dns", "grpc . unix://"+fd)
	g, err := parseGRPC(c)

	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := g.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}

type grpcDnsServiceServer struct {
	pb.UnimplementedDnsServiceServer
}

func (*grpcDnsServiceServer) Query(ctx context.Context, in *pb.DnsPacket) (*pb.DnsPacket, error) {
	msg := &dns.Msg{}
	msg.Unpack(in.GetMsg())
	answer := new(dns.Msg)
	answer.Answer = append(answer.Answer, test.A("example.org. IN A 127.0.0.1"))
	answer.SetRcode(msg, dns.RcodeSuccess)
	buf, _ := answer.Pack()
	return &pb.DnsPacket{Msg: buf}, nil
}
