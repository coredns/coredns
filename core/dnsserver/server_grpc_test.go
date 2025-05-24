package dnsserver

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

func TestNewServergRPC(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		configs []*Config
		wantErr bool
	}{
		{
			name:    "valid grpc server",
			addr:    "127.0.0.1:0",
			configs: []*Config{testConfig("grpc", testPlugin{})},
			wantErr: false,
		},
		{
			name:    "empty configs",
			addr:    "127.0.0.1:0",
			configs: []*Config{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServergRPC(tt.addr, tt.configs)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServergRPC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewServergRPC() returned nil server without error")
			}
		})
	}
}

func TestNewServergRPCWithTLS(t *testing.T) {
	config := testConfig("grpc", testPlugin{})
	config.TLSConfig = &tls.Config{
		ServerName: "example.com",
	}

	server, err := NewServergRPC("127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServergRPC() with TLS failed: %v", err)
	}

	if server.tlsConfig == nil {
		t.Error("Expected TLS config to be set")
	}

	if len(server.tlsConfig.NextProtos) == 0 || server.tlsConfig.NextProtos[0] != "h2" {
		t.Error("Expected NextProtos to include h2 for gRPC")
	}
}

func TestServergRPC_Listen(t *testing.T) {
	server, err := NewServergRPC(transport.GRPC+"://127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	listener, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}
	defer listener.Close()

	if listener == nil {
		t.Error("Listen() returned nil listener")
	}

	// Verify it's a TCP listener
	if _, ok := listener.Addr().(*net.TCPAddr); !ok {
		t.Errorf("Expected TCP listener, got %T", listener.Addr())
	}
}

func TestServergRPC_Listen_InvalidAddress(t *testing.T) {
	server, err := NewServergRPC(transport.GRPC+"://invalid:99999", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	_, err = server.Listen()
	if err == nil {
		t.Error("Listen() should fail with invalid address")
	}
}

func TestServergRPC_ListenPacket(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	conn, err := server.ListenPacket()
	if err != nil {
		t.Errorf("ListenPacket() failed: %v", err)
	}
	if conn != nil {
		t.Error("ListenPacket() should return nil for gRPC server")
	}
}

func TestServergRPC_ServePacket(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	err = server.ServePacket(nil)
	if err != nil {
		t.Errorf("ServePacket() should not return error, got: %v", err)
	}
}

func TestServergRPC_Stop(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	// Test stopping server without grpcServer initialized
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	// Test stopping after initializing grpcServer
	server.grpcServer = grpc.NewServer()
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop() with grpcServer failed: %v", err)
	}
}

func TestServergRPC_Shutdown(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	// Test shutdown without grpcServer
	err = server.Shutdown()
	if err != nil {
		t.Errorf("Shutdown() failed: %v", err)
	}

	// Test shutdown with grpcServer
	server.grpcServer = grpc.NewServer()
	err = server.Shutdown()
	if err != nil {
		t.Errorf("Shutdown() with grpcServer failed: %v", err)
	}
}

func TestServergRPC_OnStartupComplete(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:53", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	Quiet = true
	server.OnStartupComplete()

	Quiet = false
	server.OnStartupComplete()
}

func TestServergRPC_Query(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	packed, err := msg.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS message: %v", err)
	}

	dnsPacket := &pb.DnsPacket{Msg: packed}

	tcpAddr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
	p := &peer.Peer{Addr: tcpAddr}
	ctx := peer.NewContext(context.Background(), p)

	server.listenAddr = tcpAddr

	response, err := server.Query(ctx, dnsPacket)
	if err != nil {
		t.Errorf("Query() failed: %v", err)
	}

	if len(response.Msg) == 0 {
		t.Error("Query() returned empty message")
	}

	// Verify the response can be unpacked
	respMsg := new(dns.Msg)
	err = respMsg.Unpack(response.Msg)
	if err != nil {
		t.Errorf("Failed to unpack response message: %v", err)
	}
}

func TestServergRPC_Query_ErrorCases(t *testing.T) {
	server, err := NewServergRPC("127.0.0.1:0", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServergRPC() failed: %v", err)
	}

	tests := []struct {
		name    string
		ctx     context.Context
		packet  *pb.DnsPacket
		wantErr bool
	}{
		{
			name:    "invalid DNS message",
			ctx:     peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{}}),
			packet:  &pb.DnsPacket{Msg: []byte("invalid")},
			wantErr: true,
		},
		{
			name:    "no peer in context",
			ctx:     context.Background(),
			packet:  &pb.DnsPacket{Msg: []byte{}},
			wantErr: true,
		},
		{
			name:    "non-TCP peer",
			ctx:     peer.NewContext(context.Background(), &peer.Peer{Addr: &net.UDPAddr{}}),
			packet:  &pb.DnsPacket{Msg: []byte{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.Query(tt.ctx, tt.packet)
			if (err != nil) != tt.wantErr {
				t.Errorf("Query() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGRPCResponse(t *testing.T) {
	localAddr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:53")
	remoteAddr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")

	r := &gRPCresponse{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}

	if r.LocalAddr() != localAddr {
		t.Errorf("LocalAddr() = %v, want %v", r.LocalAddr(), localAddr)
	}

	if r.RemoteAddr() != remoteAddr {
		t.Errorf("RemoteAddr() = %v, want %v", r.RemoteAddr(), remoteAddr)
	}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	packed, err := msg.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS message: %v", err)
	}

	n, err := r.Write(packed)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}

	if n != len(packed) {
		t.Errorf("Write() returned %d, want %d", n, len(packed))
	}

	if r.Msg == nil {
		t.Error("Write() did not set Msg")
	}

	newMsg := new(dns.Msg)
	newMsg.SetQuestion("test.com.", dns.TypeAAAA)

	err = r.WriteMsg(newMsg)
	if err != nil {
		t.Errorf("WriteMsg() failed: %v", err)
	}

	if r.Msg != newMsg {
		t.Error("WriteMsg() did not set correct message")
	}
	if err := r.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	if err := r.TsigStatus(); err != nil {
		t.Errorf("TsigStatus() returned error: %v", err)
	}
}

func TestGRPCResponse_WriteInvalidMessage(t *testing.T) {
	r := &gRPCresponse{}

	_, err := r.Write([]byte("invalid dns message"))
	if err == nil {
		t.Error("Write() should return error for invalid DNS message")
	}
}
