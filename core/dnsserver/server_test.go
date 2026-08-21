package dnsserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type testPlugin struct{}

func (tp testPlugin) ServeDNS(_ctx context.Context, _w dns.ResponseWriter, _r *dns.Msg) (int, error) {
	return 0, nil
}

func (tp testPlugin) Name() string { return "local" }

type updateResponsePlugin struct {
	called atomic.Bool
}

func (p *updateResponsePlugin) Name() string { return "update-response" }

func (p *updateResponsePlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	p.called.Store(true)

	m := new(dns.Msg)
	m.SetReply(r)
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

func newRFC2136Update(t *testing.T, zone string) *dns.Msg {
	t.Helper()

	m := new(dns.Msg).SetUpdate(zone)
	rr, err := dns.NewRR("host." + zone + " 300 IN A 192.0.2.123")
	if err != nil {
		t.Fatalf("dns.NewRR() failed: %v", err)
	}
	m.Insert([]dns.RR{rr})
	return m
}

func mustPackRFC2136Update(t *testing.T) []byte {
	t.Helper()

	m := newRFC2136Update(t, "example.com.")
	// DNS-over-QUIC requires the DNS message ID to be zero.
	m.Id = 0

	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("dns.Msg.Pack() failed: %v", err)
	}
	return wire
}

// blockingPlugin uses sync.Mutex to simulate extended processing.
type blockingPlugin struct {
	sync.Mutex
}

func (b *blockingPlugin) Name() string { return "blocking" }

func (b *blockingPlugin) ServeDNS(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// Respond immediately to avoid waiting in dns.Exchange
	m := new(dns.Msg)
	m.SetRcodeFormatError(r)
	w.WriteMsg(m)

	b.Lock()
	defer b.Unlock()
	return dns.RcodeSuccess, nil
}

func testConfig(transport string, p plugin.Handler) *Config {
	c := &Config{
		Zone:        "example.com.",
		Transport:   transport,
		ListenHosts: []string{"127.0.0.1"},
		Port:        "53",
		Debug:       false,
		Stacktrace:  false,
	}

	c.AddPlugin(func(_next plugin.Handler) plugin.Handler { return p })
	return c
}

func TestNewServer(t *testing.T) {
	_, err := NewServer("127.0.0.1:53", []*Config{testConfig("dns", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}

	_, err = NewServergRPC("127.0.0.1:53", []*Config{testConfig("grpc", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServergRPC, got %s", err)
	}

	_, err = NewServerTLS("127.0.0.1:53", []*Config{testConfig("tls", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServerTLS, got %s", err)
	}

	_, err = NewServerQUIC("127.0.0.1:53", []*Config{testConfig("quic", testPlugin{})})
	if err != nil {
		t.Errorf("Expected no error for NewServerQUIC, got %s", err)
	}
}

func TestUpdateAdmission(t *testing.T) {
	for _, network := range []string{"udp", "tcp"} {
		for _, allow := range []bool{false, true} {
			name := network + "/default-reject"
			if allow {
				name = network + "/explicit-opt-in"
			}
			t.Run(name, func(t *testing.T) {
				handler := new(updateResponsePlugin)
				cfg := testConfig("dns", handler)
				if allow {
					cfg.AllowOpcode(dns.OpcodeUpdate)
				}

				response := exchangeWithTestServer(t, network, []*Config{cfg}, newRFC2136Update(t, "example.com."))
				wantRcode := dns.RcodeNotImplemented
				if allow {
					wantRcode = dns.RcodeSuccess
				}
				if response.Rcode != wantRcode {
					t.Fatalf("rcode = %s, want %s", dns.RcodeToString[response.Rcode], dns.RcodeToString[wantRcode])
				}
				if handler.called.Load() != allow {
					t.Fatalf("plugin called = %v, want %v", handler.called.Load(), allow)
				}
			})
		}
	}
}

func TestUpdateAdmissionIsScopedToConfig(t *testing.T) {
	dynamicHandler := new(updateResponsePlugin)
	dynamicConfig := testConfig("dns", dynamicHandler)
	dynamicConfig.Zone = "dynamic.example."
	dynamicConfig.AllowOpcode(dns.OpcodeUpdate)

	staticHandler := new(updateResponsePlugin)
	staticConfig := testConfig("dns", staticHandler)
	staticConfig.Zone = "static.example."

	response := exchangeWithTestServer(t, "udp", []*Config{dynamicConfig, staticConfig}, newRFC2136Update(t, "static.example."))
	if response.Rcode != dns.RcodeNotImplemented {
		t.Fatalf("rcode = %s, want NOTIMP", dns.RcodeToString[response.Rcode])
	}
	if dynamicHandler.called.Load() || staticHandler.called.Load() {
		t.Fatalf("UPDATE reached a plugin: dynamic=%v static=%v", dynamicHandler.called.Load(), staticHandler.called.Load())
	}
}

func TestUpdateAdmissionHeaderChecks(t *testing.T) {
	s := &Server{allowedOpcodes: map[int]struct{}{dns.OpcodeUpdate: {}}}
	header := dns.Header{
		Bits:    uint16(dns.OpcodeUpdate << 11),
		Qdcount: 1,
		Ancount: 3,
		Nscount: 3,
		Arcount: 3,
	}
	if got := s.acceptMessage(header); got != dns.MsgAccept {
		t.Fatalf("valid UPDATE action = %v, want MsgAccept", got)
	}

	header.Qdcount = 0
	if got := s.acceptMessage(header); got != dns.MsgReject {
		t.Fatalf("zero-question UPDATE action = %v, want MsgReject", got)
	}
	header.Qdcount = 2
	if got := s.acceptMessage(header); got != dns.MsgReject {
		t.Fatalf("two-question UPDATE action = %v, want MsgReject", got)
	}

	header = dns.Header{Bits: uint16(dns.OpcodeUpdate<<11) | 1<<15, Qdcount: 1}
	if got := s.acceptMessage(header); got != dns.MsgIgnore {
		t.Fatalf("UPDATE response action = %v, want MsgIgnore", got)
	}

	header = dns.Header{Bits: uint16(dns.OpcodeStatus << 11), Qdcount: 1}
	if got := s.acceptMessage(header); got != dns.MsgRejectNotImplemented {
		t.Fatalf("unregistered opcode action = %v, want MsgRejectNotImplemented", got)
	}

	queryHeader := dns.Header{Qdcount: 1, Nscount: 2}
	if got := s.acceptMessage(queryHeader); got != dns.MsgReject {
		t.Fatalf("invalid QUERY action = %v, want MsgReject", got)
	}
}

func TestUpdateAdmissionPreservesTSIGStatus(t *testing.T) {
	const (
		keyName = "update-key.example."
		secret  = "MTIzNDU2Nzg5MDEyMzQ1Ng=="
	)

	called := make(chan struct{}, 1)
	handler := tsigStatusCheckPlugin{
		t:      t,
		called: called,
		check: func(t *testing.T, status error) {
			t.Helper()
			if status != nil {
				t.Fatalf("TsigStatus() = %v, want nil", status)
			}
		},
	}
	cfg := testConfig("dns", handler)
	cfg.AllowOpcode(dns.OpcodeUpdate)
	cfg.TsigSecret = map[string]string{keyName: secret}

	request := newRFC2136Update(t, "example.com.")
	request.SetTsig(keyName, dns.HmacSHA256, 300, time.Now().Unix())
	client := &dns.Client{TsigSecret: map[string]string{keyName: secret}}
	response := exchangeWithTestServerUsingClient(t, "udp", []*Config{cfg}, request, client)
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
	}
	select {
	case <-called:
	default:
		t.Fatal("TSIG status plugin was not called")
	}
}

func exchangeWithTestServer(t *testing.T, network string, configs []*Config, request *dns.Msg) *dns.Msg {
	t.Helper()
	return exchangeWithTestServerUsingClient(t, network, configs, request, new(dns.Client))
}

func exchangeWithTestServerUsingClient(t *testing.T, network string, configs []*Config, request *dns.Msg, client *dns.Client) *dns.Msg {
	t.Helper()

	s, err := NewServer("127.0.0.1:0", configs)
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	var addr string
	switch network {
	case "udp":
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.ListenPacket() failed: %v", err)
		}
		addr = pc.LocalAddr().String()
		go func() { _ = s.ServePacket(pc) }()
		t.Cleanup(func() { _ = pc.Close() })
	case "tcp":
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen() failed: %v", err)
		}
		addr = listener.Addr().String()
		go func() { _ = s.Serve(listener) }()
		t.Cleanup(func() { _ = listener.Close() })
	default:
		t.Fatalf("unsupported network %q", network)
	}
	t.Cleanup(func() { _ = s.Stop() })

	client.Net = network
	client.Timeout = 2 * time.Second
	response, _, err := client.Exchange(request, addr)
	if err != nil {
		t.Fatalf("dns exchange failed: %v", err)
	}
	return response
}

func TestDebug(t *testing.T) {
	configNoDebug, configDebug := testConfig("dns", testPlugin{}), testConfig("dns", testPlugin{})
	configDebug.Debug = true

	s1, err := NewServer("127.0.0.1:53", []*Config{configDebug, configNoDebug})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if !s1.debug {
		t.Errorf("Expected debug mode enabled for server s1")
	}
	if !log.D.Value() {
		t.Errorf("Expected debug logging enabled")
	}

	s2, err := NewServer("127.0.0.1:53", []*Config{configNoDebug})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if s2.debug {
		t.Errorf("Expected debug mode disabled for server s2")
	}
	if log.D.Value() {
		t.Errorf("Expected debug logging disabled")
	}
}

func TestStacktrace(t *testing.T) {
	configNoStacktrace, configStacktrace := testConfig("dns", testPlugin{}), testConfig("dns", testPlugin{})
	configStacktrace.Stacktrace = true

	s1, err := NewServer("127.0.0.1:53", []*Config{configStacktrace, configStacktrace})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if !s1.stacktrace {
		t.Errorf("Expected stacktrace mode enabled for server s1")
	}

	s2, err := NewServer("127.0.0.1:53", []*Config{configNoStacktrace})
	if err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if s2.stacktrace {
		t.Errorf("Expected stacktrace disabled for server s2")
	}
}

func TestGracefulStopTimeout_Internal(t *testing.T) {
	p := new(blockingPlugin)
	cfg := testConfig("dns", p)

	s, err := NewServer("127.0.0.1:0", []*Config{cfg})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Shorten the graceful timeout
	s.graceTimeout = 500 * time.Millisecond

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket failed: %v", err)
	}
	defer pc.Close()

	go s.ServePacket(pc)
	udp := pc.LocalAddr().String()

	// Block the handler
	p.Lock()
	defer p.Unlock()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	_, err = dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("dns.Exchange failed: %v", err)
	}

	err = s.Stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestMaxTCPQueriesBoundary proves the user-visible behavior of MaxTCPQueries:
// a persistent TCP connection may serve exactly the configured number of
// queries before the server closes it.
func TestMaxTCPQueriesBoundary(t *testing.T) {
	n := 2
	config := testConfig("dns", test.ErrorHandler())
	config.MaxTCPQueries = &n

	s, err := NewServer("127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer s.Stop()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer l.Close()

	go s.Serve(l)

	conn, err := net.DialTimeout("tcp", l.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.DialTimeout failed: %v", err)
	}
	defer conn.Close()

	dnsConn := &dns.Conn{Conn: conn}

	for i := range n {
		m := new(dns.Msg)
		m.SetQuestion("example.org.", dns.TypeA)

		dnsConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := dnsConn.WriteMsg(m); err != nil {
			t.Fatalf("query %d: WriteMsg failed: %v", i, err)
		}

		dnsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := dnsConn.ReadMsg(); err != nil {
			t.Fatalf("query %d: ReadMsg failed: %v", i, err)
		}
	}

	// The connection should be closed by the server after serving n queries;
	// query n+1 must not succeed on the same connection.
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	dnsConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := dnsConn.WriteMsg(m); err == nil {
		dnsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := dnsConn.ReadMsg(); err == nil {
			t.Fatal("expected query beyond MaxTCPQueries to fail on the same connection, but it succeeded")
		}
	}
}

func BenchmarkCoreServeDNS(b *testing.B) {
	s, err := NewServer("127.0.0.1:53", []*Config{testConfig("dns", testPlugin{})})
	if err != nil {
		b.Errorf("Expected no error for NewServer, got %s", err)
	}

	ctx := context.TODO()
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("aaa.example.com.", dns.TypeTXT)

	b.ReportAllocs()

	for b.Loop() {
		s.ServeDNS(ctx, w, m)
	}
}

// recordingWriter counts the packed frames the decorated writer receives
// before forwarding them to the real writer. The decorator mints a fresh
// wrapper per packet; the frames counter is shared across them.
type recordingWriter struct {
	dns.Writer
	frames *atomic.Int64
}

func (rw *recordingWriter) Write(b []byte) (int, error) {
	rw.frames.Add(1)
	return rw.Writer.Write(b)
}

func TestUDPDecorateWriterFunc(t *testing.T) {
	cfg := testConfig("dns", test.ErrorHandler())

	frames := new(atomic.Int64)
	var gotServer atomic.Pointer[Server]
	var calls atomic.Int64
	cfg.UDPDecorateWriterFunc = func(srv *Server) dns.DecorateWriter {
		calls.Add(1)
		gotServer.Store(srv)
		return func(w dns.Writer) dns.Writer {
			return &recordingWriter{Writer: w, frames: frames}
		}
	}

	s, err := NewServer("127.0.0.1:0", []*Config{cfg})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket failed: %v", err)
	}
	defer pc.Close()

	go s.ServePacket(pc)
	defer s.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	if _, err := dns.Exchange(m, pc.LocalAddr().String()); err != nil {
		t.Fatalf("dns.Exchange failed: %v", err)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("expected UDPDecorateWriterFunc to be called once per socket, got %d", n)
	}
	if gotServer.Load() != s {
		t.Errorf("expected UDPDecorateWriterFunc to receive the serving *Server")
	}
	if frames.Load() == 0 {
		t.Errorf("expected the decorated writer to observe the response write")
	}
}
