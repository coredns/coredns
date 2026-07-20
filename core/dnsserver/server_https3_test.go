package dnsserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	testTSIGKeyNameHTTPS3     = "tsig-key."
	testTSIGSecretHTTPS3      = "MTIzNA=="
	testTSIGWrongSecretHTTPS3 = "NTY3OA=="
)

func testServerHTTPS3(t *testing.T, path string, validator func(*http.Request) bool) *http.Response {
	t.Helper()
	c := Config{
		Zone:                    "example.com.",
		Transport:               "https",
		TLSConfig:               &tls.Config{},
		ListenHosts:             []string{"127.0.0.1"},
		Port:                    "443",
		HTTPRequestValidateFunc: validator,
	}
	s, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Log(err)
		t.Fatal("could not create HTTPS3 server")
	}
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeDNSKEY)
	buf, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)

	return w.Result()
}

func TestCustomHTTP3RequestValidator(t *testing.T) {
	testCases := map[string]struct {
		path      string
		expected  int
		validator func(*http.Request) bool
	}{
		"default":                     {"/dns-query", http.StatusOK, nil},
		"custom validator":            {"/b10cada", http.StatusOK, validator},
		"no validator set":            {"/adb10c", http.StatusNotFound, nil},
		"invalid path with validator": {"/helloworld", http.StatusNotFound, validator},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			res := testServerHTTPS3(t, tc.path, tc.validator)
			if res.StatusCode != tc.expected {
				t.Error("unexpected HTTP code", res.StatusCode)
			}
			res.Body.Close()
		})
	}
}

func TestServerHTTPS3RejectsUpdate(t *testing.T) {
	handler := new(updateResponsePlugin)
	config := testConfig("https3", handler)
	config.TLSConfig = &tls.Config{}

	server, err := NewServerHTTPS3("127.0.0.1:443", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(mustPackRFC2136Update(t)))
	req.RemoteAddr = "127.0.0.1:12345"
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("ServeHTTP() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if handler.called.Load() {
		t.Fatal("RFC 2136 UPDATE reached the plugin chain")
	}
}

func TestNewServerHTTPS3WithCustomLimits(t *testing.T) {
	maxStreams := 50
	c := Config{
		Zone:             "example.com.",
		Transport:        "https3",
		TLSConfig:        &tls.Config{},
		ListenHosts:      []string{"127.0.0.1"},
		Port:             "443",
		MaxHTTPS3Streams: &maxStreams,
	}

	server, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() with custom limits failed: %v", err)
	}

	if server.maxStreams != maxStreams {
		t.Errorf("Expected maxStreams = %d, got %d", maxStreams, server.maxStreams)
	}

	expectedMaxStreams := int64(maxStreams)
	if server.quicConfig.MaxIncomingStreams != expectedMaxStreams {
		t.Errorf("Expected quicConfig.MaxIncomingStreams = %d, got %d", expectedMaxStreams, server.quicConfig.MaxIncomingStreams)
	}

	if server.quicConfig.MaxIncomingUniStreams != expectedMaxStreams {
		t.Errorf("Expected quicConfig.MaxIncomingUniStreams = %d, got %d", expectedMaxStreams, server.quicConfig.MaxIncomingUniStreams)
	}
}

func TestNewServerHTTPS3Defaults(t *testing.T) {
	c := Config{
		Zone:        "example.com.",
		Transport:   "https3",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}

	server, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() failed: %v", err)
	}

	if server.maxStreams != DefaultHTTPS3MaxStreams {
		t.Errorf("Expected default maxStreams = %d, got %d", DefaultHTTPS3MaxStreams, server.maxStreams)
	}

	expectedMaxStreams := int64(DefaultHTTPS3MaxStreams)
	if server.quicConfig.MaxIncomingStreams != expectedMaxStreams {
		t.Errorf("Expected default quicConfig.MaxIncomingStreams = %d, got %d", expectedMaxStreams, server.quicConfig.MaxIncomingStreams)
	}
}

func TestNewServerHTTPS3ZeroLimits(t *testing.T) {
	zero := 0
	c := Config{
		Zone:             "example.com.",
		Transport:        "https3",
		TLSConfig:        &tls.Config{},
		ListenHosts:      []string{"127.0.0.1"},
		Port:             "443",
		MaxHTTPS3Streams: &zero,
	}

	server, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() with zero limits failed: %v", err)
	}

	if server.maxStreams != 0 {
		t.Errorf("Expected maxStreams = 0, got %d", server.maxStreams)
	}
	// When maxStreams is 0, quicConfig should not set MaxIncomingStreams (uses QUIC default)
	if server.quicConfig.MaxIncomingStreams != 0 {
		t.Errorf("Expected quicConfig.MaxIncomingStreams = 0 (QUIC default), got %d", server.quicConfig.MaxIncomingStreams)
	}
}

func TestNewServerHTTPS3DefaultMaxHeaderBytes(t *testing.T) {
	c := Config{
		Zone:        "example.com.",
		Transport:   "https3",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}

	server, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() failed: %v", err)
	}

	if server.httpsServer.MaxHeaderBytes != DefaultHTTPS3MaxHeaderBytes {
		t.Errorf("expected MaxHeaderBytes = %d, got %d",
			DefaultHTTPS3MaxHeaderBytes,
			server.httpsServer.MaxHeaderBytes)
	}
}

func testConfigWithTSIGCheckPluginHTTPS3(t *testing.T, check func(*testing.T, error)) *Config {
	t.Helper()

	c := &Config{
		Zone:        "example.com.",
		Transport:   "https3",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
		TsigSecret: map[string]string{
			testTSIGKeyNameHTTPS3: testTSIGSecretHTTPS3,
		},
	}
	c.AddPlugin(func(_next plugin.Handler) plugin.Handler {
		return tsigStatusCheckPlugin{t: t, check: check}
	})
	return c
}

func testServerHTTPS3Msg(t *testing.T, cfg *Config, req *dns.Msg) *dns.Msg {
	t.Helper()

	s, err := NewServerHTTPS3("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS3 server:", err)
	}

	buf, err := req.Pack()
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(buf))
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected HTTP status: got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	m := new(dns.Msg)
	if err := m.Unpack(body); err != nil {
		t.Fatal(err)
	}
	return m
}

func testServerHTTPS3Raw(t *testing.T, cfg *Config, buf []byte) *dns.Msg {
	t.Helper()

	s, err := NewServerHTTPS3("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS3 server:", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(buf))
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected HTTP status: got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	m := new(dns.Msg)
	if err := m.Unpack(body); err != nil {
		t.Fatal(err)
	}
	return m
}

func forgedTSIGMsgHTTPS3() *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	m.Extra = append(m.Extra, &dns.TSIG{
		Hdr: dns.RR_Header{
			Name:   "bogus-key.",
			Rrtype: dns.TypeTSIG,
			Class:  dns.ClassANY,
			Ttl:    0,
		},
		Algorithm:  dns.HmacSHA256,
		TimeSigned: uint64(time.Now().Unix()),
		Fudge:      300,
		MACSize:    32,
		MAC:        strings.Repeat("00", 32),
		OrigId:     m.Id,
		Error:      dns.RcodeSuccess,
	})
	return m
}

func TestServeHTTP3RejectsUnsignedTSIGRequiredRequest(t *testing.T) {
	cfg := testConfigWithTSIGCheckPluginHTTPS3(t, func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("expected nil TsigStatus for unsigned request, got %v", err)
		}
	})

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	resp := testServerHTTPS3Msg(t, cfg, m)

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR response from plugin, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTP3RejectsTSIGWithUnknownKey(t *testing.T) {
	cfg := testConfigWithTSIGCheckPluginHTTPS3(t, func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, dns.ErrSecret) {
			t.Fatalf("expected dns.ErrSecret for unknown TSIG key, got %v", err)
		}
	})

	resp := testServerHTTPS3Msg(t, cfg, forgedTSIGMsgHTTPS3())
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR response from plugin, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTP3RejectsTSIGWithBadMAC(t *testing.T) {
	cfg := testConfigWithTSIGCheckPluginHTTPS3(t, func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected non-nil TsigStatus for bad TSIG MAC")
		}
	})

	buf := mustPackSignedTSIGQuery(t, testTSIGKeyNameHTTPS3, testTSIGWrongSecretHTTPS3, time.Now().Unix())
	resp := testServerHTTPS3Raw(t, cfg, buf)

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR response from plugin, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTP3AcceptsValidTSIG(t *testing.T) {
	cfg := testConfigWithTSIGCheckPluginHTTPS3(t, func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("expected nil TsigStatus for valid TSIG, got %v", err)
		}
	})

	buf := mustPackSignedTSIGQuery(t, testTSIGKeyNameHTTPS3, testTSIGSecretHTTPS3, time.Now().Unix())
	resp := testServerHTTPS3Raw(t, cfg, buf)

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR response from plugin, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTP3DoesNotLeakBodyReadError(t *testing.T) {
	c := Config{
		Zone:        "example.com.",
		Transport:   "https",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}
	s, err := NewServerHTTPS3("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatal("could not create HTTPS3 server:", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/dns-query", errReader{})
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(body)); got != "invalid request" {
		t.Fatalf("expected sanitized body %q, got %q", "invalid request", got)
	}
}

func requireHTTPS3ConnectionRejected(t *testing.T, err error) {
	t.Helper()

	var appErr *quic.ApplicationError
	if errors.As(err, &appErr) {
		if !appErr.Remote {
			t.Fatalf("connection closed with local application error: %v", err)
		}
		if appErr.ErrorCode != quic.ApplicationErrorCode(http3.ErrCodeExcessiveLoad) {
			t.Fatalf("application error code = %#x, want %#x", appErr.ErrorCode, http3.ErrCodeExcessiveLoad)
		}
		return
	}

	// An application close sent before 1-RTT keys are available is encoded as
	// the generic transport-level APPLICATION_ERROR. In that case, the peer
	// cannot observe the HTTP/3 application code or reason phrase.
	var transportErr *quic.TransportError
	if errors.As(err, &transportErr) {
		if !transportErr.Remote {
			t.Fatalf("connection closed with local transport error: %v", err)
		}
		if transportErr.ErrorCode != quic.ApplicationErrorErrorCode {
			t.Fatalf("transport error code = %#x, want APPLICATION_ERROR", transportErr.ErrorCode)
		}
		return
	}

	t.Fatalf("second connection error has unexpected type %T: %v", err, err)
}

func TestServerHTTPS3MaxConnections(t *testing.T) {
	maxConnections := 1

	config := testConfig("https3", echoPlugin{})
	config.TLSConfig = mustMakeQUICServerTLSConfig(t)
	config.MaxHTTPS3Connections = &maxConnections

	server, err := NewServerHTTPS3("127.0.0.1:0", []*Config{config})
	if err != nil {
		t.Fatalf("NewServerHTTPS3() failed: %v", err)
	}

	accepted := make(chan struct{}, 1)
	server.httpsServer.ConnContext = func(ctx context.Context, _ *quic.Conn) context.Context {
		accepted <- struct{}{}
		return ctx
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.ListenPacket() failed: %v", err)
	}
	defer pc.Close()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ServePacket(pc)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientTLS := mustMakeQUICClientTLSConfig()
	clientTLS.NextProtos = []string{"h3"}

	first, err := quic.DialAddr(ctx, pc.LocalAddr().String(), clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("first quic.DialAddr() failed: %v", err)
	}

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("first connection was not accepted by the HTTP/3 server")
	}

	second, err := quic.DialAddr(ctx, pc.LocalAddr().String(), clientTLS, &quic.Config{})
	if err == nil {
		defer second.CloseWithError(0, "")

		select {
		case <-second.Context().Done():
			err = context.Cause(second.Context())
		case <-time.After(2 * time.Second):
			t.Fatal("second connection remained open after the maximum connection count was reached")
		}
	}
	if err == nil {
		t.Fatal("second connection closed without reporting an error")
	}
	requireHTTPS3ConnectionRejected(t, err)

	if err := first.CloseWithError(0, ""); err != nil {
		t.Fatalf("first.CloseWithError() failed: %v", err)
	}
	select {
	case <-first.Context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first connection did not close")
	}

	stopErrCh := make(chan error, 1)
	go func() {
		stopErrCh <- server.Stop()
	}()

	select {
	case err := <-stopErrCh:
		if err != nil {
			t.Fatalf("Stop() failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return")
	}

	select {
	case err := <-serveErrCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ServePacket() error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServePacket() did not stop after Stop()")
	}
}
