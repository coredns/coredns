package grpcproxy

import (
	"context"
	"testing"
	"time"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGrpcProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	opts := &GrpcOpts{}
	p := New("", opts, metrics)
	msg, _ := new(dns.Msg).Pack()
	mock := newMockDNSServiceClient(&pb.DnsPacket{Msg: msg}, nil)
	p.client = mock
	defer p.Stop()

	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	if _, err := p.Query(context.TODO(), state); err != nil {
		t.Error("Expecting a response")
	}
}

func TestTLSGrpcProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	filename, rmFunc, err := test.TempFile("", aCert)
	if err != nil {
		t.Errorf("Error saving file : %s", err)
		return
	}
	defer rmFunc()
	tls, _ := tls.NewTLSClientConfig(filename)
	// ignore error as the certificate is known valid
	opts := &GrpcOpts{TLSConfig: tls}
	p := New("", opts, metrics)
	msg, _ := new(dns.Msg).Pack()
	mock := newMockDNSServiceClient(&pb.DnsPacket{Msg: msg}, nil)
	p.client = mock
	defer p.Stop()

	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	if _, err := p.Query(context.TODO(), state); err != nil {
		t.Error("Expecting a response")
	}
}

func TestUnavailableGrpcProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	opts := &GrpcOpts{}
	p := New("", opts, metrics)
	msg, _ := new(dns.Msg).Pack()
	err := status.Error(codes.Unavailable, "")
	mock := newMockDNSServiceClient(&pb.DnsPacket{Msg: msg}, err)
	p.client = mock
	defer p.Stop()

	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	if _, err := p.Query(context.TODO(), state); err != nil {
		st := status.Convert(err)
		if st.Code() != codes.Unavailable {
			t.Errorf("Expecting an unavailable error when querying gRPC client with invalid hostname : %s", err)
		}
	}
}

func TestHealthcheckGrpcProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	opts := &GrpcOpts{}
	p := New("", opts, metrics)
	msg, _ := new(dns.Msg).Pack()
	err := status.Error(codes.Unavailable, "")
	mock := newMockDNSServiceClient(&pb.DnsPacket{Msg: msg}, err)
	p.client = mock
	p.probe.Start(100 * time.Millisecond)
	defer p.Stop()

	p.Healthcheck()
	time.Sleep(200 * time.Millisecond)
	if !p.Down(1) {
		t.Error("Expecting a down service")
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
func (m mockDNSServiceClient) Watch(ctx context.Context, opts ...grpc.CallOption) (pb.DnsService_WatchClient, error) {
	return nil, nil
}

const (
	aCert = `-----BEGIN CERTIFICATE-----
	MIIDlDCCAnygAwIBAgIJAPaRnBJUE/FVMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTcxMTI0MTM0OTQ3WhcNMTgxMTI0MTM0OTQ3WjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAuTDeAoWS6tdZVcp/Vh3FlagbC+9Ohi5VjRXgkpcn9JopbcF5s2jpl1v+
cRpqkrmNNKLh8qOhmgdZQdh185VNe/iZ94H42qwKZ48vvnC5hLkk3MdgUT2ewgup
vZhy/Bb1bX+buCWkQa1u8SIilECMIPZHhBP4TuBUKJWK8bBEFAeUnxB5SCkX+un4
pctRlcfg8sX/ghADnp4e//YYDqex+1wQdFqM5zWhWDZAzc5Kdkyy9r+xXNfo4s1h
fI08f6F4skz1koxG2RXOzQ7OK4YxFwT2J6V72iyzUIlRGZTbYDvair/zm1kjTF1R
B1B+XLJF9oIB4BMZbekf033ZVaQ8YwIDAQABo4GGMIGDMDMGA1UdEQQsMCqHBH8A
AAGHBDR3AQGHBDR3AQCHBDR3KmSHBDR3KGSHBDR3KmWHBDR3KtIwHQYDVR0OBBYE
FFAEccLm7D/rN3fEe1fwzH7p0spAMB8GA1UdIwQYMBaAFFAEccLm7D/rN3fEe1fw
zH7p0spAMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAF4zqaucNcK2
GwYfijwbbtgMqPEvbReUEXsC65riAPjksJQ9L2YxQ7K0RIugRizuD1DNQam+FSb0
cZEMEKzvMUIexbhZNFINWXY2X9yUS/oZd5pWP0WYIhn6qhmLvzl9XpxNPVzBXYWe
duMECCigU2x5tAGmFa6g/pXXOoZCBRzFXwXiuNhSyhJEEwODjLZ6vgbySuU2jso3
va4FKFDdVM16s1/RYOK5oM48XytCMB/JoYoSJHPfpt8LpVNAQEHMvPvHwuZBON/z
q8HFtDjT4pBpB8AfuzwtUZ/zJ5atwxa5+ahcqRnK2kX2RSINfyEy43FZjLlvjcGa
UIRTUJK1JKg=
-----END CERTIFICATE-----`
)
