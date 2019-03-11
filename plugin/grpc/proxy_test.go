package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/pb"
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
		"up": {
			p:       &Proxy{fails: 1},
			res:     &dns.Msg{},
			wantErr: false,
		},
		"down": {
			p:       &Proxy{fails: 3},
			res:     &dns.Msg{},
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
			t.Parallel()
			var mock *mockDNSServiceClient
			if tt.res != nil {
				msg, err := tt.res.Pack()
				if err != nil {
					t.Fatalf("Error packing response: %s", err.Error())
				}
				mock = newMockDNSServiceClient(&pb.DnsPacket{Msg: msg}, nil)
			} else {
				mock = newMockDNSServiceClient(nil, errors.New("server error"))
			}
			tt.p.client = mock

			_, err := tt.p.query(context.TODO(), new(dns.Msg))
			if err != nil && !tt.wantErr {
				t.Fatalf("Error query(): %s", err.Error())
			}

			if tt.p.down(2) && !tt.wantErr {
				t.Fatal("Proxy shouldn't be down")
			}
		})
	}
}

type mockDNSServiceClient struct {
	dnsPacket *pb.DnsPacket
	err       error
}

func newMockDNSServiceClient(dnsPacket *pb.DnsPacket, err error) *mockDNSServiceClient {
	return &mockDNSServiceClient{dnsPacket: dnsPacket, err: err}
}
func (m mockDNSServiceClient) Query(ctx context.Context, in *pb.DnsPacket, opts ...grpc.CallOption) (*pb.DnsPacket, error) {
	return m.dnsPacket, m.err
}
