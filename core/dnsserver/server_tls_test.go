package dnsserver

import (
	"errors"
	"net"
	"testing"
)

type stubListener struct {
	addr net.Addr
}

func (l *stubListener) Accept() (net.Conn, error) {
	return nil, errors.New("test listener closed")
}

func (l *stubListener) Close() error {
	return nil
}

func (l *stubListener) Addr() net.Addr {
	if l.addr != nil {
		return l.addr
	}
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func TestServerTLSSetsTsigSecret(t *testing.T) {
	server, err := NewServerTLS("tls://127.0.0.1:0", []*Config{testConfig("tls", testPlugin{})})
	if err != nil {
		t.Fatalf("NewServerTLS() failed: %v", err)
	}

	server.tsigSecret = map[string]string{
		"test.": "abcd",
	}

	l := &stubListener{}

	err = server.Serve(l)
	if err == nil {
		t.Fatal("expected Serve() to return from stub listener")
	}

	if server.server[tcp] == nil {
		t.Fatal("expected tcp server to be initialized")
	}

	if server.server[tcp].TsigSecret == nil {
		t.Fatal("expected TsigSecret to be propagated")
	}

	if got := server.server[tcp].TsigSecret["test."]; got != "abcd" {
		t.Fatalf("expected tsig secret %q, got %q", "abcd", got)
	}
}
