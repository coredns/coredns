package test

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/pb"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var grpcCorefile = `grpc://.:0 {
	whoami
}`

var grpcLimitCorefile = `grpc://.:0 {
	grpc_server {
		max_streams 2
	}
	whoami
}`

var grpcConnectionLimitCorefile = `grpc://.:0 {
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	grpc_server {
		max_connections 2
	}
	whoami
}`

func TestGrpc(t *testing.T) {
	corefile := grpcCorefile

	g, _, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer g.Stop()

	conn, err := grpc.NewClient(tcp, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}
	defer conn.Close()

	client := pb.NewDnsServiceClient(conn)

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()

	reply, err := client.Query(context.TODO(), &pb.DnsPacket{Msg: msg})
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	d := new(dns.Msg)
	err = d.Unpack(reply.GetMsg())
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", d.Rcode)
	}

	if len(d.Extra) != 2 {
		t.Errorf("Expected 2 RRs in additional section, but got %d", len(d.Extra))
	}
}

// TestGRPCWithLimits tests that the server starts and works with configured limits
func TestGRPCWithLimits(t *testing.T) {
	g, _, tcp, err := CoreDNSServerAndPorts(grpcLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer g.Stop()

	conn, err := grpc.NewClient(tcp, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}
	defer conn.Close()

	client := pb.NewDnsServiceClient(conn)

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()

	reply, err := client.Query(context.Background(), &pb.DnsPacket{Msg: msg})
	if err != nil {
		t.Fatalf("Query failed: %s", err)
	}

	d := new(dns.Msg)
	if err := d.Unpack(reply.GetMsg()); err != nil {
		t.Fatalf("Failed to unpack: %s", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", d.Rcode)
	}
}

// TestGRPCConnectionLimit tests that connection limits are enforced
func TestGRPCConnectionLimit(t *testing.T) {
	g, _, tcp, err := CoreDNSServerAndPorts(grpcConnectionLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer g.Stop()

	const maxConns = 2

	// Create TLS connections to hold them open
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	conns := make([]net.Conn, 0, maxConns+1)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Open connections up to the limit - these should succeed
	for i := range maxConns {
		conn, err := tls.Dial("tcp", tcp, tlsConfig)
		if err != nil {
			t.Fatalf("Connection %d failed (should succeed): %v", i+1, err)
		}
		conns = append(conns, conn)
	}

	// Try to open more connections beyond the limit - should timeout
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 100 * time.Millisecond},
		"tcp", tcp, tlsConfig,
	)
	if err == nil {
		conn.Close()
		t.Fatal("Connection beyond limit should have timed out")
	}

	// Close one connection and verify a new one can be established
	conns[0].Close()
	conns = conns[1:]

	time.Sleep(10 * time.Millisecond)

	conn, err = tls.Dial("tcp", tcp, tlsConfig)
	if err != nil {
		t.Fatalf("Connection after freeing slot failed: %v", err)
	}
	conns = append(conns, conn)
}

// TestGRPCTLSWithLimits tests that gRPC with TLS starts and works with configured limits
func TestGRPCTLSWithLimits(t *testing.T) {
	g, _, tcp, err := CoreDNSServerAndPorts(grpcConnectionLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer g.Stop()

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(tcp, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}
	defer conn.Close()

	client := pb.NewDnsServiceClient(conn)

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()

	reply, err := client.Query(context.Background(), &pb.DnsPacket{Msg: msg})
	if err != nil {
		t.Fatalf("Query failed: %s", err)
	}

	d := new(dns.Msg)
	if err := d.Unpack(reply.GetMsg()); err != nil {
		t.Fatalf("Failed to unpack: %s", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", d.Rcode)
	}
}
