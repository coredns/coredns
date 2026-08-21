package dnsserver

import (
	"errors"
	"net"
	"testing"

	"github.com/miekg/dns"
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

func TestServerTLSUpdateAdmission(t *testing.T) {
	for _, allow := range []bool{false, true} {
		name := "default-reject"
		if allow {
			name = "explicit-opt-in"
		}
		t.Run(name, func(t *testing.T) {
			config := testConfig("tls", testPlugin{})
			if allow {
				config.AllowOpcode(dns.OpcodeUpdate)
			}
			server, err := NewServerTLS("tls://127.0.0.1:0", []*Config{config})
			if err != nil {
				t.Fatalf("NewServerTLS() failed: %v", err)
			}
			if err := server.Serve(&stubListener{}); err == nil {
				t.Fatal("expected Serve() to return from stub listener")
			}

			accept := server.server[tcp].MsgAcceptFunc
			if accept == nil {
				t.Fatal("TLS server did not initialize MsgAcceptFunc")
			}
			header := dns.Header{Bits: uint16(dns.OpcodeUpdate << 11), Qdcount: 1}
			want := dns.MsgRejectNotImplemented
			if allow {
				want = dns.MsgAccept
			}
			if got := accept(header); got != want {
				t.Fatalf("UPDATE action = %v, want %v", got, want)
			}
		})
	}
}

func TestServerSetsMaxTCPQueries(t *testing.T) {
	n := 128

	t.Run("default is unlimited", func(t *testing.T) {
		server, err := NewServer("127.0.0.1:0", []*Config{testConfig("dns", testPlugin{})})
		if err != nil {
			t.Fatalf("NewServer() failed: %v", err)
		}

		if err := server.Serve(&stubListener{}); err == nil {
			t.Fatal("expected Serve() to return from stub listener")
		}

		if got := server.server[tcp].MaxTCPQueries; got != -1 {
			t.Fatalf("expected default MaxTCPQueries -1, got %d", got)
		}
	})

	t.Run("configured value reaches the TCP server", func(t *testing.T) {
		config := testConfig("dns", testPlugin{})
		config.MaxTCPQueries = &n

		server, err := NewServer("127.0.0.1:0", []*Config{config})
		if err != nil {
			t.Fatalf("NewServer() failed: %v", err)
		}

		if err := server.Serve(&stubListener{}); err == nil {
			t.Fatal("expected Serve() to return from stub listener")
		}

		if got := server.server[tcp].MaxTCPQueries; got != n {
			t.Fatalf("expected MaxTCPQueries %d, got %d", n, got)
		}
	})

	t.Run("configured value reaches the TLS server", func(t *testing.T) {
		config := testConfig("tls", testPlugin{})
		config.MaxTCPQueries = &n

		server, err := NewServerTLS("tls://127.0.0.1:0", []*Config{config})
		if err != nil {
			t.Fatalf("NewServerTLS() failed: %v", err)
		}

		if err := server.Serve(&stubListener{}); err == nil {
			t.Fatal("expected Serve() to return from stub listener")
		}

		if got := server.server[tcp].MaxTCPQueries; got != n {
			t.Fatalf("expected MaxTCPQueries %d, got %d", n, got)
		}
	})
}
