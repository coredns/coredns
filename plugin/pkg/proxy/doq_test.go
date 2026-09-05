package proxy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

type doqTestHandler func(int64, *quic.Conn, *quic.Stream, *dns.Msg) error

type doqTestServer struct {
	listener *quic.Listener
	handler  doqTestHandler

	ctx        context.Context
	cancel     context.CancelFunc
	acceptDone chan struct{}
	errors     chan error
	closeOnce  sync.Once
	wg         sync.WaitGroup
	mu         sync.Mutex
	conns      map[*quic.Conn]struct{}
	accepted   atomic.Int64
	streams    atomic.Int64
}

func newDoQTestServer(t *testing.T, handler doqTestHandler) (*doqTestServer, *tls.Config) {
	t.Helper()
	serverTLS, clientTLS := makeDoQTestTLSConfigs(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, &quic.Config{MaxIncomingStreams: 256})
	if err != nil {
		t.Fatalf("quic.ListenAddr() failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &doqTestServer{
		listener:   listener,
		handler:    handler,
		ctx:        ctx,
		cancel:     cancel,
		acceptDone: make(chan struct{}),
		errors:     make(chan error, 64),
		conns:      make(map[*quic.Conn]struct{}),
	}
	go s.serve()
	t.Cleanup(s.close)
	return s, clientTLS
}

func (s *doqTestServer) addr() string { return s.listener.Addr().String() }

func (s *doqTestServer) serve() {
	defer close(s.acceptDone)
	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			return
		}
		connNumber := s.accepted.Add(1)
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Go(func() { s.serveConn(connNumber, conn) })
	}
}

func (s *doqTestServer) serveConn(connNumber int64, conn *quic.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()
	for {
		stream, err := conn.AcceptStream(s.ctx)
		if err != nil {
			return
		}
		s.streams.Add(1)
		s.wg.Go(func() {
			if err := s.serveStream(connNumber, conn, stream); err != nil {
				select {
				case s.errors <- err:
				default:
				}
			}
		})
	}
}

func (s *doqTestServer) serveStream(connNumber int64, conn *quic.Conn, stream *quic.Stream) error {
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	wire, err := readDOQMessage(stream)
	if err != nil {
		return fmt.Errorf("read query: %w", err)
	}
	if err := expectDOQFIN(stream); err != nil {
		return fmt.Errorf("read query FIN: %w", err)
	}
	query := new(dns.Msg)
	if err := query.Unpack(wire); err != nil {
		return fmt.Errorf("unpack query: %w", err)
	}
	return s.handler(connNumber, conn, stream, query)
}

func (s *doqTestServer) close() {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.listener.Close()
		<-s.acceptDone

		s.mu.Lock()
		connections := make([]*quic.Conn, 0, len(s.conns))
		for conn := range s.conns {
			connections = append(connections, conn)
		}
		s.mu.Unlock()
		for _, conn := range connections {
			_ = conn.CloseWithError(0, "test shutdown")
		}

		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
}

func makeDoQTestTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() failed: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "doq.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"doq.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() failed: %v", err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey}
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() failed: %v", err)
	}
	roots.AddCert(parsed)
	return &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{doqALPN},
		}, &tls.Config{
			RootCAs:    roots,
			ServerName: "doq.test",
		}
}

func writeDoQTestResponse(stream *quic.Stream, response *dns.Msg) error {
	wire, err := response.Pack()
	if err != nil {
		return err
	}
	if err := writeDOQMessage(stream, wire); err != nil {
		return err
	}
	return stream.Close()
}

func replyToDoQTestQuery(stream *quic.Stream, query *dns.Msg) error {
	response := new(dns.Msg)
	response.SetReply(query)
	return writeDoQTestResponse(stream, response)
}

func doqTestRequest(name string, id uint16) request.Request {
	query := new(dns.Msg)
	query.SetQuestion(name, dns.TypeA)
	query.Id = id
	return request.Request{Req: query}
}

func TestProxyDoQExchange(t *testing.T) {
	type observation struct {
		id           uint16
		alpn         string
		hasKeepalive bool
	}
	observed := make(chan observation, 1)
	server, clientTLS := newDoQTestServer(t, func(_ int64, conn *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		obs := observation{id: query.Id, alpn: conn.ConnectionState().TLS.NegotiatedProtocol}
		if opt := query.IsEdns0(); opt != nil {
			for _, option := range opt.Option {
				obs.hasKeepalive = obs.hasKeepalive || option.Option() == dns.EDNS0TCPKEEPALIVE
			}
		}
		observed <- obs
		response := new(dns.Msg)
		response.SetReply(query)
		record, err := dns.NewRR("example.org. 60 IN A 192.0.2.1")
		if err != nil {
			return err
		}
		response.Answer = []dns.RR{record}
		return writeDoQTestResponse(stream, response)
	})

	p := NewProxy("TestProxyDoQExchange", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	defer p.Stop()

	state := doqTestRequest("example.org.", 0x1234)
	state.Req.SetEdns0(1232, false)
	state.Req.IsEdns0().Option = append(state.Req.IsEdns0().Option, &dns.EDNS0_TCP_KEEPALIVE{Code: dns.EDNS0TCPKEEPALIVE, Timeout: 10})
	response, localAddr, proto, err := p.Connect(context.Background(), state, Options{ForceTCP: true})
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	if response.Id != 0x1234 {
		t.Fatalf("response ID = %d, want %d", response.Id, 0x1234)
	}
	if state.Req.Id != 0x1234 {
		t.Fatalf("request ID was mutated: got %d", state.Req.Id)
	}
	if len(state.Req.IsEdns0().Option) != 1 {
		t.Fatal("the downstream EDNS TCP keepalive option was mutated")
	}
	if proto != "udp" {
		t.Fatalf("reported protocol = %q, want udp", proto)
	}
	if _, ok := localAddr.(*net.UDPAddr); !ok {
		t.Fatalf("local address type = %T, want *net.UDPAddr", localAddr)
	}
	if len(response.Answer) != 1 || response.Answer[0].String() != "example.org.\t60\tIN\tA\t192.0.2.1" {
		t.Fatalf("unexpected answer: %v", response.Answer)
	}

	obs := <-observed
	if obs.id != 0 {
		t.Errorf("upstream query ID = %d, want 0", obs.id)
	}
	if obs.alpn != doqALPN {
		t.Errorf("negotiated ALPN = %q, want %q", obs.alpn, doqALPN)
	}
	if obs.hasKeepalive {
		t.Error("upstream query retained the EDNS TCP keepalive option")
	}
}

func TestProxyDoQVerifiesServerName(t *testing.T) {
	server, clientTLS := newDoQTestServer(t, func(_ int64, _ *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		return replyToDoQTestQuery(stream, query)
	})

	badTLS := clientTLS.Clone()
	badTLS.ServerName = "wrong.test"
	p := NewProxy("TestProxyDoQVerifiesServerName", server.addr(), transport.QUIC)
	p.SetTLSConfig(badTLS)
	p.SetReadTimeout(time.Second)
	defer p.Stop()

	_, _, _, err := p.Connect(context.Background(), doqTestRequest("example.org.", 1), Options{})
	if err == nil {
		t.Fatal("Connect() succeeded with the wrong TLS server name")
	}
	var hostnameError x509.HostnameError
	if !errors.As(err, &hostnameError) {
		t.Fatalf("Connect() error = %T %v, want x509.HostnameError", err, err)
	}
}

func TestProxyDoQSourceAddress(t *testing.T) {
	remoteAddress := make(chan net.Addr, 1)
	server, clientTLS := newDoQTestServer(t, func(_ int64, conn *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		remoteAddress <- conn.RemoteAddr()
		return replyToDoQTestQuery(stream, query)
	})

	p := NewProxy("TestProxyDoQSourceAddress", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	p.SetLocalAddress(net.ParseIP("127.0.0.2"))
	defer p.Stop()

	_, localAddress, _, err := p.Connect(context.Background(), doqTestRequest("example.org.", 1), Options{})
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	localUDP, ok := localAddress.(*net.UDPAddr)
	if !ok {
		t.Fatalf("local address type = %T, want *net.UDPAddr", localAddress)
	}
	if got := localUDP.IP.String(); got != "127.0.0.2" {
		t.Fatalf("local source address = %s, want 127.0.0.2", got)
	}
	remote := <-remoteAddress
	remoteUDP, ok := remote.(*net.UDPAddr)
	if !ok {
		t.Fatalf("remote address type = %T, want *net.UDPAddr", remote)
	}
	if got := remoteUDP.IP.String(); got != "127.0.0.2" {
		t.Fatalf("server observed source address = %s, want 127.0.0.2", got)
	}
}

func TestProxyDoQReusesOneConnectionForConcurrentQueries(t *testing.T) {
	const queries = 16
	var active atomic.Int64
	var maxActive atomic.Int64
	var arrived atomic.Int64
	release := make(chan struct{})
	server, clientTLS := newDoQTestServer(t, func(_ int64, _ *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		if arrived.Add(1) == queries {
			close(release)
		}
		select {
		case <-release:
		case <-time.After(3 * time.Second):
			return errors.New("concurrent queries did not arrive on time")
		}
		return replyToDoQTestQuery(stream, query)
	})

	p := NewProxy("TestProxyDoQConcurrent", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	p.SetReadTimeout(4 * time.Second)
	defer p.Stop()

	var wg sync.WaitGroup
	errs := make(chan error, queries)
	for i := range queries {
		wg.Go(func() {
			state := doqTestRequest(fmt.Sprintf("q%d.example.", i), uint16(i+1))
			response, _, _, err := p.Connect(context.Background(), state, Options{})
			if err == nil && response.Id != uint16(i+1) {
				err = fmt.Errorf("response ID = %d, want %d", response.Id, i+1)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Connect() failed: %v", err)
		}
	}
	if got := server.accepted.Load(); got != 1 {
		t.Errorf("accepted connections = %d, want 1", got)
	}
	if got := server.streams.Load(); got != queries {
		t.Errorf("accepted streams = %d, want %d", got, queries)
	}
	if got := maxActive.Load(); got != queries {
		t.Errorf("maximum concurrent streams = %d, want %d", got, queries)
	}
}

func TestProxyDoQCancellationDoesNotCloseConnection(t *testing.T) {
	cancelledWrite := make(chan error, 1)
	server, clientTLS := newDoQTestServer(t, func(_ int64, _ *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		if query.Question[0].Name == "slow.example." {
			time.Sleep(250 * time.Millisecond)
			response := new(dns.Msg)
			response.SetReply(query)
			err := writeDoQTestResponse(stream, response)
			cancelledWrite <- err
			return nil
		}
		return replyToDoQTestQuery(stream, query)
	})

	p := NewProxy("TestProxyDoQCancellation", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	p.SetReadTimeout(time.Second)
	defer p.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, _, err := p.Connect(ctx, doqTestRequest("slow.example.", 1), Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow Connect() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("canceled Connect() returned after %s", elapsed)
	}

	response, _, _, err := p.Connect(context.Background(), doqTestRequest("fast.example.", 2), Options{})
	if err != nil {
		t.Fatalf("Connect() after cancellation failed: %v", err)
	}
	if response.Id != 2 {
		t.Fatalf("response ID = %d, want 2", response.Id)
	}
	if got := server.accepted.Load(); got != 1 {
		t.Fatalf("connections after stream cancellation = %d, want 1", got)
	}
	select {
	case err := <-cancelledWrite:
		if err == nil {
			t.Error("server write on the canceled stream unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe the canceled stream")
	}
}

func TestProxyDoQReplacesClosedConnection(t *testing.T) {
	server, clientTLS := newDoQTestServer(t, func(connNumber int64, conn *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		if err := replyToDoQTestQuery(stream, query); err != nil {
			return err
		}
		if connNumber == 1 {
			go func() {
				time.Sleep(10 * time.Millisecond)
				_ = conn.CloseWithError(0, "rotate test connection")
			}()
		}
		return nil
	})

	p := NewProxy("TestProxyDoQReplacesClosed", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	p.SetReadTimeout(time.Second)
	defer p.Stop()

	if _, _, _, err := p.Connect(context.Background(), doqTestRequest("first.example.", 1), Options{}); err != nil {
		t.Fatalf("first Connect() failed: %v", err)
	}
	p.doq.mu.Lock()
	first := p.doq.current
	p.doq.mu.Unlock()
	if first == nil {
		t.Fatal("first QUIC connection was not cached")
	}
	select {
	case <-first.conn.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("server did not close the first QUIC connection")
	}

	if _, _, _, err := p.Connect(context.Background(), doqTestRequest("second.example.", 2), Options{}); err != nil {
		t.Fatalf("second Connect() failed: %v", err)
	}
	if got := server.accepted.Load(); got != 2 {
		t.Fatalf("accepted connections = %d, want 2", got)
	}
}

func TestProxyDoQProtocolErrorRetiresConnection(t *testing.T) {
	server, clientTLS := newDoQTestServer(t, func(connNumber int64, _ *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
		response := new(dns.Msg)
		response.SetReply(query)
		if connNumber == 1 {
			response.Id = 1
		}
		return writeDoQTestResponse(stream, response)
	})

	p := NewProxy("TestProxyDoQProtocolError", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	p.SetReadTimeout(time.Second)
	defer p.Stop()

	_, _, _, err := p.Connect(context.Background(), doqTestRequest("bad.example.", 10), Options{})
	if !errors.Is(err, errDoQProtocol) {
		t.Fatalf("first Connect() error = %v, want DoQ protocol error", err)
	}
	response, _, _, err := p.Connect(context.Background(), doqTestRequest("good.example.", 11), Options{})
	if err != nil {
		t.Fatalf("Connect() after protocol error failed: %v", err)
	}
	if response.Id != 11 {
		t.Fatalf("response ID = %d, want 11", response.Id)
	}
	if got := server.accepted.Load(); got != 2 {
		t.Fatalf("accepted connections = %d, want 2", got)
	}
}

func TestDoQHealthCheck(t *testing.T) {
	query := make(chan *dns.Msg, 1)
	server, clientTLS := newDoQTestServer(t, func(_ int64, _ *quic.Conn, stream *quic.Stream, msg *dns.Msg) error {
		query <- msg.Copy()
		return replyToDoQTestQuery(stream, msg)
	})

	p := NewProxy("TestDoQHealth", server.addr(), transport.QUIC)
	p.SetTLSConfig(clientTLS)
	defer p.Stop()
	hc := p.GetHealthchecker()
	hc.SetDomain("health.example.")
	hc.SetRecursionDesired(false)
	if err := hc.Check(p); err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	msg := <-query
	if len(msg.Question) != 1 || msg.Question[0].Name != "health.example." || msg.Question[0].Qtype != dns.TypeNS {
		t.Fatalf("unexpected health query: %v", msg.Question)
	}
	if msg.RecursionDesired {
		t.Error("health query unexpectedly requested recursion")
	}
	if msg.Id != 0 {
		t.Errorf("health query ID = %d, want 0", msg.Id)
	}
}

func TestProxyDoQRejectsZoneTransferBeforeDial(t *testing.T) {
	p := NewProxy("TestProxyDoQRejectsZoneTransfer", "127.0.0.1:1", transport.QUIC)
	defer p.Stop()
	for _, qtype := range []uint16{dns.TypeAXFR, dns.TypeIXFR} {
		query := new(dns.Msg)
		query.SetQuestion("example.org.", qtype)
		_, _, _, err := p.Connect(context.Background(), request.Request{Req: query}, Options{})
		if !errors.Is(err, ErrUnsupportedRequest) {
			t.Errorf("Connect(%s) error = %v, want ErrUnsupportedRequest", dns.TypeToString[qtype], err)
		}
	}
	if got := len(p.doq.connections); got != 0 {
		t.Fatalf("zone transfer opened %d DoQ connections, want 0", got)
	}
}

func TestDoQConnectionExpiryAndMaxAge(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Proxy)
	}{
		{
			name: "expire",
			configure: func(p *Proxy) {
				p.SetExpire(20 * time.Millisecond)
			},
		},
		{
			name: "max age",
			configure: func(p *Proxy) {
				p.SetExpire(time.Hour)
				p.SetMaxAge(20 * time.Millisecond)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, clientTLS := newDoQTestServer(t, func(_ int64, _ *quic.Conn, stream *quic.Stream, query *dns.Msg) error {
				return replyToDoQTestQuery(stream, query)
			})
			p := NewProxy("TestDoQLifetime", server.addr(), transport.QUIC)
			p.SetTLSConfig(clientTLS)
			p.SetReadTimeout(time.Second)
			tc.configure(p)
			defer p.Stop()

			if _, _, _, err := p.Connect(context.Background(), doqTestRequest("first.example.", 1), Options{}); err != nil {
				t.Fatalf("first Connect() failed: %v", err)
			}
			time.Sleep(30 * time.Millisecond)
			if _, _, _, err := p.Connect(context.Background(), doqTestRequest("second.example.", 2), Options{}); err != nil {
				t.Fatalf("second Connect() failed: %v", err)
			}
			if got := server.accepted.Load(); got != 2 {
				t.Fatalf("accepted connections = %d, want 2", got)
			}
		})
	}
}

func TestDoQStopCancelsDial(t *testing.T) {
	packetConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("net.ListenUDP() failed: %v", err)
	}
	defer packetConn.Close()

	p := NewProxy("TestDoQStopCancelsDial", packetConn.LocalAddr().String(), transport.QUIC)
	p.SetTLSConfig(&tls.Config{ServerName: "doq.test"})
	result := make(chan error, 1)
	go func() {
		_, _, _, err := p.Connect(context.Background(), doqTestRequest("example.org.", 1), Options{})
		result <- err
	}()

	time.Sleep(50 * time.Millisecond)
	p.Stop()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Connect() unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop() did not cancel the in-progress DoQ dial")
	}
	p.Stop()
	_, _, _, err = p.Connect(context.Background(), doqTestRequest("example.org.", 2), Options{})
	if err == nil || err.Error() != ErrTransportStopped {
		t.Fatalf("Connect() after Stop() error = %v, want %q", err, ErrTransportStopped)
	}
}

func TestDoQFraming(t *testing.T) {
	var framed bytes.Buffer
	if err := writeDOQMessage(&framed, []byte{1, 2, 3}); err != nil {
		t.Fatalf("writeDOQMessage() failed: %v", err)
	}
	if want := []byte{0, 3, 1, 2, 3}; !bytes.Equal(framed.Bytes(), want) {
		t.Fatalf("framed message = %v, want %v", framed.Bytes(), want)
	}
	message, err := readDOQMessage(&framed)
	if err != nil {
		t.Fatalf("readDOQMessage() failed: %v", err)
	}
	if !bytes.Equal(message, []byte{1, 2, 3}) {
		t.Fatalf("message = %v, want [1 2 3]", message)
	}

	invalid := [][]byte{
		{},
		{0},
		{0, 0},
		{0, 2, 1},
	}
	for _, wire := range invalid {
		if _, err := readDOQMessage(bytes.NewReader(wire)); !errors.Is(err, errDoQProtocol) {
			t.Errorf("readDOQMessage(%v) error = %v, want protocol error", wire, err)
		}
	}
	if err := expectDOQFIN(bytes.NewReader(nil)); err != nil {
		t.Errorf("expectDOQFIN(empty) failed: %v", err)
	}
	if err := expectDOQFIN(bytes.NewReader([]byte{1})); !errors.Is(err, errDoQProtocol) {
		t.Errorf("expectDOQFIN(extra byte) error = %v, want protocol error", err)
	}
}
