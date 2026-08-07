package dnsserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/transport"

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

func TestAcquireQUICWorkerWaitsForSlot(t *testing.T) {
	pool := make(chan struct{}, 1)
	pool <- struct{}{}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- acquireQUICWorker(ctx, pool)
	}()

	select {
	case <-done:
		t.Fatal("acquireQUICWorker returned before a slot was released")
	default:
	}

	<-pool

	got := <-done
	if !got {
		t.Fatal("expected acquireQUICWorker to succeed after slot release")
	}
}

func TestAcquireQUICWorkerReturnsFalseOnCancelledContext(t *testing.T) {
	pool := make(chan struct{}, 1)
	pool <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := acquireQUICWorker(ctx, pool); got {
		t.Fatal("expected acquireQUICWorker to return false when context is cancelled")
	}
}

func TestDoQWriterTsigStatusReturnsStoredStatus(t *testing.T) {
	want := errors.New("bad tsig")

	w := &DoQWriter{
		tsigStatus: want,
	}

	if got := w.TsigStatus(); got != want {
		t.Fatalf("TsigStatus() = %v, want %v", got, want)
	}
}

func TestServerQUIC_ServeQUIC_TSIGBadSigSetsTsigStatus(t *testing.T) {
	const keyName = "tsig-key."
	const clientSecret = "MTIzNDU2Nzg5MDEyMzQ1Ng=="
	const serverSecret = "QUJDREVGR0hJSktMTU5PUA=="

	called := make(chan struct{}, 1)

	config := testConfig("quic", tsigStatusCheckPlugin{
		t:      t,
		called: called,
		check: func(t *testing.T, got error) {
			t.Helper()
			if got == nil {
				t.Fatal("TsigStatus() = nil, want non-nil for bad TSIG MAC")
			}
			if errors.Is(got, dns.ErrSecret) {
				t.Fatalf("TsigStatus() = %v, want signature verification error, not ErrSecret", got)
			}
			if errors.Is(got, dns.ErrTime) {
				t.Fatalf("TsigStatus() = %v, want signature verification error, not ErrTime", got)
			}
		},
	})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	server.tsigSecret = map[string]string{
		keyName: serverSecret,
	}

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()

	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() failed: %v", err)
	}
	defer conn.CloseWithError(DoQCodeNoError, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() failed: %v", err)
	}

	wire := mustPackSignedTSIGQuery(t, keyName, clientSecret, time.Now().Unix())

	if _, err := stream.Write(AddPrefix(wire)); err != nil {
		t.Fatalf("stream.Write() failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream.Close() failed: %v", err)
	}

	respWire, err := readDOQMessage(stream)
	if err != nil {
		t.Fatalf("readDOQMessage() failed: %v", err)
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(respWire); err != nil {
		t.Fatalf("response unpack failed: %v", err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("ServeDNS() was not called")
	}
}

func TestServerQUIC_ServeQUICRejectsUpdate(t *testing.T) {
	handler := new(updateResponsePlugin)
	config := testConfig("quic", handler)
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()
	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() failed: %v", err)
	}
	defer conn.CloseWithError(DoQCodeNoError, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() failed: %v", err)
	}
	if err := stream.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() failed: %v", err)
	}
	if _, err := stream.Write(AddPrefix(mustPackRFC2136Update(t))); err != nil {
		t.Fatalf("stream.Write() failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream.Close() failed: %v", err)
	}

	_, err = readDOQMessage(stream)
	if err == nil {
		t.Fatal("DoQ server accepted an RFC 2136 UPDATE")
	}
	var applicationErr *quic.ApplicationError
	if !errors.As(err, &applicationErr) {
		t.Fatalf("readDOQMessage() error = %T %v, want QUIC application error", err, err)
	}
	if applicationErr.ErrorCode != DoQCodeProtocolError {
		t.Fatalf("QUIC application error code = %d, want %d", applicationErr.ErrorCode, DoQCodeProtocolError)
	}
	if handler.called.Load() {
		t.Fatal("RFC 2136 UPDATE reached the plugin chain")
	}
}

// echoPlugin answers every query with a minimal reply. It is used as a
// negative control to prove a normal DoQ query is still served after the
// per-stream read deadline was introduced.
type echoPlugin struct{}

func (echoPlugin) Name() string { return "echo" }

func (echoPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

// TestServerQUIC_ServeQUIC_StalledStreamDoesNotStarveWorkerPool is a
// regression test for the DoQ stream/worker-pool starvation DoS
// (GHSA-f2c9-fp4w-rhw6): a client that opens a stream but never finishes
// sending its query used to block a worker from streamProcessPool forever,
// because readDOQMessage was called with no read deadline. With the worker
// pool shrunk to a single slot, a stalled stream would previously prevent
// any other query from ever being served. The per-stream read deadline must
// free the worker so an unrelated, well-behaved client is still served.
func TestServerQUIC_ServeQUIC_StalledStreamDoesNotStarveWorkerPool(t *testing.T) {
	config := testConfig("quic", echoPlugin{})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)
	// A single worker means a stalled stream, absent a read deadline, would
	// hold the only worker and starve every subsequent connection.
	workerPoolSize := 1
	config.MaxQUICWorkerPoolSize = &workerPoolSize

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}
	// Keep the test fast: the stalled stream must time out quickly.
	server.ReadTimeout = 250 * time.Millisecond

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()

	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connection 1: open a stream and send a length prefix but never the
	// message body, stalling the server's readDOQMessage. This grabs the
	// only worker.
	stallConn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() for stalled conn failed: %v", err)
	}
	defer stallConn.CloseWithError(DoQCodeNoError, "")

	stallStream, err := stallConn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() for stalled stream failed: %v", err)
	}
	// Announce a 100-byte message but send nothing more, so the server
	// blocks reading the body until the read deadline fires.
	if _, err := stallStream.Write([]byte{0x00, 0x64}); err != nil {
		t.Fatalf("stalled stream.Write() failed: %v", err)
	}

	// Connection 2: a well-behaved client. Before the fix this query would
	// never be answered because the single worker is stuck on the stalled
	// stream; after the fix the stalled worker is freed once the read
	// deadline fires and this query succeeds.
	normalConn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() for normal conn failed: %v", err)
	}
	defer normalConn.CloseWithError(DoQCodeNoError, "")

	normalStream, err := normalConn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() for normal stream failed: %v", err)
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	q.Id = 0
	wire, err := q.Pack()
	if err != nil {
		t.Fatalf("dns.Msg.Pack() failed: %v", err)
	}
	if _, err := normalStream.Write(AddPrefix(wire)); err != nil {
		t.Fatalf("normal stream.Write() failed: %v", err)
	}
	if err := normalStream.Close(); err != nil {
		t.Fatalf("normal stream.Close() failed: %v", err)
	}

	respCh := make(chan error, 1)
	go func() {
		_, rerr := readDOQMessage(normalStream)
		respCh <- rerr
	}()

	select {
	case rerr := <-respCh:
		if rerr != nil {
			t.Fatalf("normal query was not served: readDOQMessage() error = %v", rerr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("normal query was not served within 5s: stalled stream starved the worker pool")
	}
}

func mustMakeQUICServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() failed: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           nil,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() failed: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"doq"},
	}
}

func mustMakeQUICClientTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"doq"},
	}
}

func TestServerQUIC_ServeQUIC_TSIGValidSigLeavesTsigStatusNil(t *testing.T) {
	const keyName = "tsig-key."
	const secret = "MTIzNDU2Nzg5MDEyMzQ1Ng=="

	called := make(chan struct{}, 1)

	config := testConfig("quic", tsigStatusCheckPlugin{
		t:      t,
		called: called,
		check: func(t *testing.T, got error) {
			t.Helper()
			if got != nil {
				t.Fatalf("TsigStatus() = %v, want nil for valid TSIG MAC", got)
			}
		},
	})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	server.tsigSecret = map[string]string{
		keyName: secret,
	}

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()

	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() failed: %v", err)
	}
	defer conn.CloseWithError(DoQCodeNoError, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() failed: %v", err)
	}

	wire := mustPackSignedTSIGQuery(t, keyName, secret, time.Now().Unix())

	if _, err := stream.Write(AddPrefix(wire)); err != nil {
		t.Fatalf("stream.Write() failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream.Close() failed: %v", err)
	}

	respWire, err := readDOQMessage(stream)
	if err != nil {
		t.Fatalf("readDOQMessage() failed: %v", err)
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(respWire); err != nil {
		t.Fatalf("response unpack failed: %v", err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("ServeDNS() was not called")
	}
}

// connectionStateCapturePlugin records the *tls.ConnectionState observed via
// the dns.ConnectionStater interface implemented by the DoQ response writer.
type connectionStateCapturePlugin struct {
	t      *testing.T
	called chan struct{}
	state  chan *tls.ConnectionState
}

func (p connectionStateCapturePlugin) Name() string { return "connection-state-capture" }

func (p connectionStateCapturePlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	p.t.Helper()

	if cs, ok := w.(dns.ConnectionStater); ok {
		p.state <- cs.ConnectionState()
	} else {
		p.state <- nil
	}
	if p.called != nil {
		p.called <- struct{}{}
	}

	m := new(dns.Msg)
	m.SetReply(r)
	if err := w.WriteMsg(m); err != nil {
		p.t.Fatalf("WriteMsg() failed: %v", err)
	}
	return dns.RcodeSuccess, nil
}

func TestServerQUIC_ServeQUIC_ConnectionStateExposesSNI(t *testing.T) {
	const sni = "doq.example.com"

	called := make(chan struct{}, 1)
	stateCh := make(chan *tls.ConnectionState, 1)

	config := testConfig("quic", connectionStateCapturePlugin{
		t:      t,
		called: called,
		state:  stateCh,
	})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()

	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTLS := mustMakeQUICClientTLSConfig()
	clientTLS.ServerName = sni

	conn, err := quic.DialAddr(ctx, pc.LocalAddr().String(), clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr() failed: %v", err)
	}
	defer conn.CloseWithError(DoQCodeNoError, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync() failed: %v", err)
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	q.Id = 0
	wire, err := q.Pack()
	if err != nil {
		t.Fatalf("dns.Msg.Pack() failed: %v", err)
	}

	if _, err := stream.Write(AddPrefix(wire)); err != nil {
		t.Fatalf("stream.Write() failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream.Close() failed: %v", err)
	}

	if _, err := readDOQMessage(stream); err != nil {
		t.Fatalf("readDOQMessage() failed: %v", err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("ServeDNS() was not called")
	}

	select {
	case state := <-stateCh:
		if state == nil {
			t.Fatal("ConnectionState() = nil, want non-nil TLS state for DoQ request")
		}
		if state.ServerName != sni {
			t.Errorf("ConnectionState().ServerName = %q, want %q", state.ServerName, sni)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive connection state from plugin")
	}
}

func TestServerQUICServeQUICDefaultMaxConnections(t *testing.T) {
	const maxConnections = 200

	config := testConfig("quic", echoPlugin{})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)

	server, err := NewServerQUIC(transport.QUIC+"://127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerQUIC() failed: %v", err)
	}

	pc, err := server.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServeQUIC()
	}()

	defer func() {
		_ = server.Stop()
		select {
		case <-serveErrCh:
		case <-time.After(2 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := func(conn *quic.Conn) error {
		stream, err := conn.OpenStreamSync(ctx)
		if err != nil {
			return err
		}

		q := new(dns.Msg)
		q.SetQuestion("example.com.", dns.TypeA)
		q.Id = 0
		wire, err := q.Pack()
		if err != nil {
			return err
		}
		if _, err := stream.Write(AddPrefix(wire)); err != nil {
			return err
		}
		if err := stream.Close(); err != nil {
			return err
		}
		_, err = readDOQMessage(stream)
		return err
	}

	connections := make([]*quic.Conn, 0, maxConnections)
	defer func() {
		for _, conn := range connections {
			_ = conn.CloseWithError(DoQCodeNoError, "")
		}
	}()

	addr := pc.LocalAddr().String()
	for i := range maxConnections {
		conn, err := quic.DialAddr(ctx, addr, mustMakeQUICClientTLSConfig(), &quic.Config{})
		if err != nil {
			t.Fatalf("quic.DialAddr() for connection %d failed: %v", i+1, err)
		}
		connections = append(connections, conn)

		// Receiving a response proves ServeQUIC accepted the connection and
		// started serveQUICConnection, which keeps its connection slot held.
		if err := query(conn); err != nil {
			t.Fatalf("query on connection %d failed: %v", i+1, err)
		}
	}

	overflow, err := quic.DialAddr(ctx, addr, mustMakeQUICClientTLSConfig(), &quic.Config{})
	if err == nil {
		err = query(overflow)
	}
	if err == nil {
		_ = overflow.CloseWithError(DoQCodeNoError, "")
		t.Fatalf("connection %d was served; want it rejected by the default connection limit", maxConnections+1)
	}

	if overflow != nil {
		select {
		case <-overflow.Context().Done():
			err = context.Cause(overflow.Context())
		case <-time.After(2 * time.Second):
		}
		_ = overflow.CloseWithError(DoQCodeNoError, "")
	}
	if err == nil || !strings.Contains(err.Error(), "too many connections") {
		t.Fatalf("connection %d rejection error = %v, want %q", maxConnections+1, err, "too many connections")
	}

	// Releasing one accepted connection must release its semaphore slot.
	_ = connections[0].CloseWithError(DoQCodeNoError, "")
	connections = connections[1:]

	deadline := time.Now().Add(5 * time.Second)
	for {
		replacement, dialErr := quic.DialAddr(ctx, addr, mustMakeQUICClientTLSConfig(), &quic.Config{})
		if dialErr == nil {
			if queryErr := query(replacement); queryErr == nil {
				connections = append(connections, replacement)
				break
			}
			_ = replacement.CloseWithError(DoQCodeNoError, "")
		}

		if time.Now().After(deadline) {
			t.Fatal("replacement connection was not served after a connection slot was released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
