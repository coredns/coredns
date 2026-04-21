package dnsserver

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
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
