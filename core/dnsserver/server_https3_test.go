package dnsserver

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/miekg/dns"
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
