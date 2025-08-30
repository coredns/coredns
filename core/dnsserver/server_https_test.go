package dnsserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

var (
	validPath = regexp.MustCompile("^/(dns-query|(?P<uuid>[0-9a-f]+))$")
	validator = func(r *http.Request) bool { return validPath.MatchString(r.URL.Path) }
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
	c.AddPlugin(func(next plugin.Handler) plugin.Handler { return p })
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
