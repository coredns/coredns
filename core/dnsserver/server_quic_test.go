package dnsserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

func TestNewServerQUIC(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		configs []*Config
		wantErr bool
	}{
		{
			name:    "valid quic server",
			addr:    "127.0.0.1:0",
			configs: []*Config{testConfig("quic", testPlugin{})},
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
			server, err := NewServerQUIC(tt.addr, tt.configs)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServerQUIC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewServerQUIC() returned nil server without error")
			}
		})
	}
}

func TestNewServerQUICWithTLS(t *testing.T) {
	config := testConfig("quic", testPlugin{})
	config.TLSConfig = &tls.Config{
		ServerName: "example.com",
	}

	server, err := NewServerQUIC("127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() with TLS failed: %v", err)
	}

	if server.tlsConfig == nil {
		t.Error("Expected TLS config to be set")
	}

	if len(server.tlsConfig.NextProtos) == 0 || server.tlsConfig.NextProtos[0] != "doq" {
		t.Error("Expected NextProtos to include doq for QUIC")
	}
}

func TestNewServerQUICWithCustomLimits(t *testing.T) {
	config := testConfig("quic", testPlugin{})
	maxStreams := 100
	workerPoolSize := 50
	config.MaxQUICStreams = &maxStreams
	config.MaxQUICWorkerPoolSize = &workerPoolSize

	server, err := NewServerQUIC("127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() with custom limits failed: %v", err)
	}

	if server.maxStreams != maxStreams {
		t.Errorf("Expected maxStreams = %d, got %d", maxStreams, server.maxStreams)
	}

	if cap(server.streamProcessPool) != workerPoolSize {
		t.Errorf("Expected streamProcessPool capacity = %d, got %d", workerPoolSize, cap(server.streamProcessPool))
	}

	expectedMaxStreams := int64(maxStreams)
	if server.quicConfig.MaxIncomingStreams != expectedMaxStreams {
		t.Errorf("Expected quicConfig.MaxIncomingStreams = %d, got %d", expectedMaxStreams, server.quicConfig.MaxIncomingStreams)
	}

	if server.quicConfig.MaxIncomingUniStreams != expectedMaxStreams {
		t.Errorf("Expected quicConfig.MaxIncomingUniStreams = %d, got %d", expectedMaxStreams, server.quicConfig.MaxIncomingUniStreams)
	}
}

func TestNewServerQUICDefaults(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:0", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	if server.maxStreams != DefaultMaxQUICStreams {
		t.Errorf("Expected default maxStreams = %d, got %d", DefaultMaxQUICStreams, server.maxStreams)
	}

	if cap(server.streamProcessPool) != DefaultQUICStreamWorkers {
		t.Errorf("Expected default streamProcessPool capacity = %d, got %d", DefaultQUICStreamWorkers, cap(server.streamProcessPool))
	}

	if !server.quicConfig.Allow0RTT {
		t.Error("Expected Allow0RTT to be true by default")
	}
}

func TestServerQUIC_ServeAndListen(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:0", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	// Test Serve - should return nil for QUIC (not used)
	err = server.Serve(nil)
	if err != nil {
		t.Errorf("Serve() should return nil for QUIC server, got: %v", err)
	}

	// Test Listen - should return nil for QUIC (not used)
	listener, err := server.Listen()
	if err != nil {
		t.Errorf("Listen() should return nil error for QUIC server, got: %v", err)
	}
	if listener != nil {
		t.Error("Listen() should return nil listener for QUIC server")
	}
}

func TestServerQUIC_OnStartupComplete(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:53", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	Quiet = true
	server.OnStartupComplete()

	Quiet = false
	server.OnStartupComplete()
}

func TestServerQUIC_Stop(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:0", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	err = server.Stop()
	if err != nil {
		t.Errorf("Stop() without listener should not error, got: %v", err)
	}
}

func TestServerQUIC_CloseQUICConn(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:0", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	server.closeQUICConn(nil, DoQCodeNoError)
}

func TestServerQUIC_IsExpectedErr(t *testing.T) {
	server, err := NewServerQUIC("127.0.0.1:0", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "server closed error",
			err:      quic.ErrServerClosed,
			expected: true,
		},
		{
			name:     "application error code 2",
			err:      &quic.ApplicationError{ErrorCode: 2},
			expected: true,
		},
		{
			name:     "application error code 1",
			err:      &quic.ApplicationError{ErrorCode: 1},
			expected: false,
		},
		{
			name:     "idle timeout error",
			err:      &quic.IdleTimeoutError{},
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.isExpectedErr(tt.err)
			if result != tt.expected {
				t.Errorf("isExpectedErr(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestValidRequest(t *testing.T) {
	tests := []struct {
		name     string
		setupMsg func() *dns.Msg
		valid    bool
	}{
		{
			name: "valid request",
			setupMsg: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.com.", dns.TypeA)
				m.Id = 0
				return m
			},
			valid: true,
		},
		{
			name: "non-zero message ID",
			setupMsg: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.com.", dns.TypeA)
				m.Id = 1234
				return m
			},
			valid: false,
		},
		{
			name: "with EDNS TCP keepalive",
			setupMsg: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.com.", dns.TypeA)
				m.Id = 0
				opt := &dns.OPT{
					Hdr: dns.RR_Header{
						Name:   ".",
						Rrtype: dns.TypeOPT,
						Class:  4096,
						Ttl:    0,
					},
					Option: []dns.EDNS0{
						&dns.EDNS0_TCP_KEEPALIVE{
							Code:    dns.EDNS0TCPKEEPALIVE,
							Timeout: 300,
						},
					},
				}
				m.Extra = append(m.Extra, opt)
				return m
			},
			valid: false,
		},
		{
			name: "with other EDNS options",
			setupMsg: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.com.", dns.TypeA)
				m.Id = 0
				opt := &dns.OPT{
					Hdr: dns.RR_Header{
						Name:   ".",
						Rrtype: dns.TypeOPT,
						Class:  4096,
						Ttl:    0,
					},
					Option: []dns.EDNS0{
						&dns.EDNS0_NSID{
							Code: dns.EDNS0NSID,
							Nsid: "test",
						},
					},
				}
				m.Extra = append(m.Extra, opt)
				return m
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.setupMsg()
			result := validRequest(msg)
			if result != tt.valid {
				t.Errorf("validRequest() = %v, want %v", result, tt.valid)
			}
		})
	}
}

func TestReadDOQMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantMsg []byte
		wantErr bool
	}{
		{
			name:    "valid message",
			input:   []byte{0x00, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05},
			wantMsg: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			wantErr: false,
		},
		{
			name:    "zero length message",
			input:   []byte{0x00, 0x00},
			wantMsg: nil,
			wantErr: true,
		},
		{
			name:    "incomplete length prefix",
			input:   []byte{0x00},
			wantMsg: nil,
			wantErr: true,
		},
		{
			name:    "incomplete message",
			input:   []byte{0x00, 0x05, 0x01, 0x02},
			wantMsg: []byte{0x01, 0x02, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   []byte{},
			wantMsg: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.input)
			msg, err := readDOQMessage(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("readDOQMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !bytes.Equal(msg, tt.wantMsg) {
				t.Errorf("readDOQMessage() msg = %v, want %v", msg, tt.wantMsg)
			}
		})
	}
}

func TestDoQWriter(t *testing.T) {
	mockStream := &mockQUICStream{}
	localAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:53")
	remoteAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")

	writer := &DoQWriter{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		stream:     mockStream,
	}

	if writer.LocalAddr() != localAddr {
		t.Errorf("LocalAddr() = %v, want %v", writer.LocalAddr(), localAddr)
	}

	if writer.RemoteAddr() != remoteAddr {
		t.Errorf("RemoteAddr() = %v, want %v", writer.RemoteAddr(), remoteAddr)
	}

	testData := []byte("test message")
	n, err := writer.Write(testData)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}

	expectedLen := len(testData) + 2 // +2 for length prefix
	if n != expectedLen {
		t.Errorf("Write() returned %d, want %d", n, expectedLen)
	}

	// Verify the written data includes length prefix
	written := mockStream.writtenData
	if len(written) != expectedLen {
		t.Errorf("Expected written data length %d, got %d", expectedLen, len(written))
	}

	// Check length prefix
	expectedLength := uint16(len(testData))
	actualLength := binary.BigEndian.Uint16(written[:2])
	if actualLength != expectedLength {
		t.Errorf("Expected length prefix %d, got %d", expectedLength, actualLength)
	}

	// Check message content
	if !bytes.Equal(written[2:], testData) {
		t.Errorf("Expected message content %v, got %v", testData, written[2:])
	}

	// Test WriteMsg method
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Id = 0

	mockStream.reset()
	err = writer.WriteMsg(msg)
	if err != nil {
		t.Errorf("WriteMsg() failed: %v", err)
	}

	if !mockStream.closed {
		t.Error("WriteMsg() should close the stream")
	}

	if err := writer.TsigStatus(); err != nil {
		t.Errorf("TsigStatus() returned error: %v", err)
	}
}

func TestAddPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "empty message",
			input:    []byte{},
			expected: []byte{0x00, 0x00},
		},
		{
			name:     "short message",
			input:    []byte{0x01, 0x02},
			expected: []byte{0x00, 0x02, 0x01, 0x02},
		},
		{
			name:     "longer message",
			input:    []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			expected: []byte{0x00, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddPrefix(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("AddPrefix() = %v, want %v", result, tt.expected)
			}
		})
	}
}

type mockQUICStream struct {
	writtenData []byte
	closed      bool
}

func (m *mockQUICStream) Write(data []byte) (int, error) {
	m.writtenData = append(m.writtenData, data...)
	return len(data), nil
}

func (m *mockQUICStream) Read([]byte) (int, error) { return 0, io.EOF }

func (m *mockQUICStream) Close() error {
	m.closed = true
	return nil
}

func (m *mockQUICStream) reset() {
	m.writtenData = nil
	m.closed = false
}

// Minimal implementation of other required methods
func (m *mockQUICStream) StreamID() quic.StreamID          { return 0 }
func (m *mockQUICStream) SetReadDeadline(time.Time) error  { return nil }
func (m *mockQUICStream) SetWriteDeadline(time.Time) error { return nil }
func (m *mockQUICStream) SetDeadline(time.Time) error      { return nil }
func (m *mockQUICStream) Context() context.Context         { return context.Background() }
func (m *mockQUICStream) CancelWrite(quic.StreamErrorCode) {}
func (m *mockQUICStream) CancelRead(quic.StreamErrorCode)  {}
