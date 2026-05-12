package dnsserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

var (
	validPath = regexp.MustCompile("^/(dns-query|(?P<uuid>[0-9a-f]+))$")
	validator = func(r *http.Request) bool { return validPath.MatchString(r.URL.Path) }
)

const (
	testTSIGKeyName     = "tsig-key."
	testTSIGSecret      = "MTIzNA=="
	testTSIGWrongSecret = "NTY3OA=="
)

func testServerHTTPS(t *testing.T, path string, validator func(*http.Request) bool) *http.Response {
	t.Helper()
	c := Config{
		Zone:                    "example.com.",
		Transport:               "https",
		TLSConfig:               &tls.Config{},
		ListenHosts:             []string{"127.0.0.1"},
		Port:                    "443",
		HTTPRequestValidateFunc: validator,
	}
	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Log(err)
		t.Fatal("could not create HTTPS server")
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

func TestCustomHTTPRequestValidator(t *testing.T) {
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
			res := testServerHTTPS(t, tc.path, tc.validator)
			if res.StatusCode != tc.expected {
				t.Error("unexpected HTTP code", res.StatusCode)
			}
			res.Body.Close()
		})
	}
}

func TestNewServerHTTPSWithCustomLimits(t *testing.T) {
	maxConnections := 100
	c := Config{
		Zone:                "example.com.",
		Transport:           "https",
		TLSConfig:           &tls.Config{},
		ListenHosts:         []string{"127.0.0.1"},
		Port:                "443",
		MaxHTTPSConnections: &maxConnections,
	}

	server, err := NewServerHTTPS("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS() with custom limits failed: %v", err)
	}

	if server.maxConnections != maxConnections {
		t.Errorf("Expected maxConnections = %d, got %d", maxConnections, server.maxConnections)
	}
}

func TestNewServerHTTPSDefaults(t *testing.T) {
	c := Config{
		Zone:        "example.com.",
		Transport:   "https",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}

	server, err := NewServerHTTPS("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS() failed: %v", err)
	}

	if server.maxConnections != DefaultHTTPSMaxConnections {
		t.Errorf("Expected default maxConnections = %d, got %d", DefaultHTTPSMaxConnections, server.maxConnections)
	}
}

func TestNewServerHTTPSZeroLimits(t *testing.T) {
	zero := 0
	c := Config{
		Zone:                "example.com.",
		Transport:           "https",
		TLSConfig:           &tls.Config{},
		ListenHosts:         []string{"127.0.0.1"},
		Port:                "443",
		MaxHTTPSConnections: &zero,
	}

	server, err := NewServerHTTPS("127.0.0.1:443", []*Config{&c})
	if err != nil {
		t.Fatalf("NewServerHTTPS() with zero limits failed: %v", err)
	}

	if server.maxConnections != 0 {
		t.Errorf("Expected maxConnections = 0, got %d", server.maxConnections)
	}
}

type contextCapturingPlugin struct {
	capturedContext  context.Context
	contextCancelled bool
}

func (p *contextCapturingPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	p.capturedContext = ctx
	select {
	case <-ctx.Done():
		p.contextCancelled = true
	default:
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (p *contextCapturingPlugin) Name() string { return "context_capturing" }

func testConfigWithPlugin(p *contextCapturingPlugin) *Config {
	c := &Config{
		Zone:        "example.com.",
		Transport:   "https",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}
	c.AddPlugin(func(_next plugin.Handler) plugin.Handler { return p })
	return c
}

func TestDoHWriterLaddrFromConnContext(t *testing.T) {
	capturer := &addrCapturingPlugin{}
	cfg := testConfigWithHandler(capturer)

	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS server:", err)
	}
	s.listenAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	buf, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a PROXY protocol destination that differs from listenAddr.
	ppDst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}

	r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
	ctx := context.WithValue(r.Context(), connAddrKey{}, ppDst)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if !capturer.called {
		t.Fatal("plugin was not called")
	}
	if capturer.localAddr == nil {
		t.Fatal("DoHWriter.laddr is nil")
	}
	if capturer.localAddr.String() != ppDst.String() {
		t.Errorf("expected laddr %s (PP destination), got %s", ppDst, capturer.localAddr)
	}
}

func TestDoHWriterLaddrFallback(t *testing.T) {
	capturer := &addrCapturingPlugin{}
	cfg := testConfigWithHandler(capturer)

	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS server:", err)
	}
	s.listenAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	buf, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}

	// No connAddrKey in context; should fall back to s.listenAddr.
	r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if !capturer.called {
		t.Fatal("plugin was not called")
	}
	if capturer.localAddr == nil {
		t.Fatal("DoHWriter.laddr is nil")
	}
	if capturer.localAddr.String() != s.listenAddr.String() {
		t.Errorf("expected fallback laddr %s, got %s", s.listenAddr, capturer.localAddr)
	}
}

type addrCapturingPlugin struct {
	called     bool
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (p *addrCapturingPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	p.called = true
	p.localAddr = w.LocalAddr()
	p.remoteAddr = w.RemoteAddr()
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (p *addrCapturingPlugin) Name() string { return "addr_capturing" }

func testConfigWithHandler(h plugin.Handler) *Config {
	c := &Config{
		Zone:        "example.com.",
		Transport:   "https",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
	}
	c.AddPlugin(func(_next plugin.Handler) plugin.Handler { return h })
	return c
}

func TestHTTPRequestContextPropagation(t *testing.T) {
	plugin := &contextCapturingPlugin{}

	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{testConfigWithPlugin(plugin)})
	if err != nil {
		t.Fatal("could not create HTTPS server:", err)
	}

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	buf, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("context values propagation", func(t *testing.T) {
		contextValue := "test-request-id"

		r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
		ctx := context.WithValue(r.Context(), Key{}, contextValue)
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		s.ServeHTTP(w, r)

		if plugin.capturedContext == nil {
			t.Fatal("No context received in plugin")
		}

		if val := plugin.capturedContext.Value(Key{}); val != s.Server {
			t.Error("Server key not properly set in context")
		}

		if httpReq, ok := plugin.capturedContext.Value(HTTPRequestKey{}).(*http.Request); !ok {
			t.Error("HTTPRequestKey not found in context")
		} else if httpReq != r {
			t.Error("HTTPRequestKey contains different request than expected")
		}
	})

	t.Run("plugins can access HTTP request details", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
		r.Header.Set("User-Agent", "my-doh-client/2.1")
		r.Header.Set("X-Forwarded-For", "10.10.10.10")
		r.Header.Set("Accept", "application/dns-message")
		r.RemoteAddr = "10.10.10.100:45678"
		w := httptest.NewRecorder()

		s.ServeHTTP(w, r)

		if plugin.capturedContext == nil {
			t.Fatal("No context received in plugin")
		}

		httpReq, ok := plugin.capturedContext.Value(HTTPRequestKey{}).(*http.Request)
		if !ok {
			t.Fatal("HTTPRequestKey not found in context")
		}

		if httpReq.Method != "POST" {
			t.Errorf("Plugin expected POST method, got %s", httpReq.Method)
		}

		if ua := httpReq.Header.Get("User-Agent"); ua != "my-doh-client/2.1" {
			t.Errorf("Plugin expected User-Agent 'my-doh-client/2.1', got %s", ua)
		}

		if xff := httpReq.Header.Get("X-Forwarded-For"); xff != "10.10.10.10" {
			t.Errorf("Plugin expected X-Forwarded-For '10.10.10.10', got %s", xff)
		}

		if accept := httpReq.Header.Get("Accept"); accept != "application/dns-message" {
			t.Errorf("Plugin expected Accept 'application/dns-message', got %s", accept)
		}

		if httpReq.RemoteAddr != "10.10.10.100:45678" {
			t.Errorf("Plugin expected RemoteAddr '10.10.10.100:45678', got %s", httpReq.RemoteAddr)
		}

		if loopValue := plugin.capturedContext.Value(LoopKey{}); loopValue != 0 {
			t.Errorf("Expected LoopKey value 0, got %v", loopValue)
		}
	})

	t.Run("context cancellation propagation", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
		ctx, cancel := context.WithCancel(r.Context())
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		cancel()
		s.ServeHTTP(w, r)

		if plugin.capturedContext == nil {
			t.Fatal("No context received in plugin")
		}

		if !plugin.contextCancelled {
			t.Error("Context cancellation was not detected in plugin")
		}

		if err := plugin.capturedContext.Err(); err == nil {
			t.Error("Expected context to be cancelled, but it wasn't")
		}
	})

	t.Run("context timeout propagation", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/dns-query", io.NopCloser(bytes.NewReader(buf)))
		ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond)
		defer cancel()
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		s.ServeHTTP(w, r)

		if plugin.capturedContext == nil {
			t.Fatal("No context received in plugin")
		}

		if deadline, ok := plugin.capturedContext.Deadline(); !ok {
			t.Error("Expected context to have a deadline")
		} else if deadline.IsZero() {
			t.Error("Context deadline is zero")
		}
	})
}

type tsigStatusPlugin struct{}

func (p *tsigStatusPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	switch {
	case r.IsTsig() == nil:
		m.Rcode = dns.RcodeRefused
	case w.TsigStatus() != nil:
		m.Rcode = dns.RcodeNotAuth
	default:
		m.Rcode = dns.RcodeSuccess
	}

	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

func (p *tsigStatusPlugin) Name() string { return "tsig_status" }

func testConfigWithTSIGStatusPlugin() *Config {
	c := &Config{
		Zone:        "example.com.",
		Transport:   "https",
		TLSConfig:   &tls.Config{},
		ListenHosts: []string{"127.0.0.1"},
		Port:        "443",
		TsigSecret: map[string]string{
			testTSIGKeyName: testTSIGSecret,
		},
	}
	c.AddPlugin(func(_next plugin.Handler) plugin.Handler { return &tsigStatusPlugin{} })
	return c
}

func testServerHTTPSMsg(t *testing.T, cfg *Config, req *dns.Msg) *dns.Msg {
	t.Helper()

	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS server:", err)
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

func testServerHTTPSRaw(t *testing.T, cfg *Config, buf []byte) *dns.Msg {
	t.Helper()

	s, err := NewServerHTTPS("127.0.0.1:443", []*Config{cfg})
	if err != nil {
		t.Fatal("could not create HTTPS server:", err)
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

func forgedTSIGMsg() *dns.Msg {
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

func mustSignedTSIGQueryBytes(t *testing.T, keyName, secret string) []byte {
	t.Helper()
	return mustPackSignedTSIGQuery(t, keyName, secret, time.Now().Unix())
}

func TestServeHTTPRejectsUnsignedTSIGRequiredRequest(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	resp := testServerHTTPSMsg(t, testConfigWithTSIGStatusPlugin(), m)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("expected REFUSED for unsigned request, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTPRejectsTSIGWithUnknownKey(t *testing.T) {
	resp := testServerHTTPSMsg(t, testConfigWithTSIGStatusPlugin(), forgedTSIGMsg())

	if resp.Rcode != dns.RcodeNotAuth {
		t.Fatalf("expected NOTAUTH for unknown TSIG key, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTPRejectsTSIGWithBadMAC(t *testing.T) {
	buf := mustSignedTSIGQueryBytes(t, testTSIGKeyName, testTSIGWrongSecret)

	resp := testServerHTTPSRaw(t, testConfigWithTSIGStatusPlugin(), buf)
	if resp.Rcode != dns.RcodeNotAuth {
		t.Fatalf("expected NOTAUTH for bad TSIG MAC, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestServeHTTPAcceptsValidTSIG(t *testing.T) {
	buf := mustSignedTSIGQueryBytes(t, testTSIGKeyName, testTSIGSecret)

	resp := testServerHTTPSRaw(t, testConfigWithTSIGStatusPlugin(), buf)
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR for valid TSIG, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestDoHWriterTsigStatusReturnsStoredStatus(t *testing.T) {
	dw := &DoHWriter{tsigStatus: dns.ErrSecret}
	if dw.TsigStatus() != dns.ErrSecret {
		t.Fatal("expected TsigStatus to return stored tsigStatus")
	}
}
