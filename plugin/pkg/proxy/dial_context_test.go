package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestConnectTLSHandshakeContext(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			started := make(chan struct{})
			closed := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					closed <- err
					return
				}
				defer conn.Close()
				// A watchdog bounds the old implementation without waiting for its 30s dial timeout.
				conn.SetReadDeadline(time.Now().Add(time.Second))
				var first [1]byte
				if _, err := io.ReadFull(conn, first[:]); err != nil {
					closed <- err
					return
				}
				close(started)
				_, err = io.Copy(io.Discard, conn)
				closed <- err
			}()

			ctx, cancel := context.WithCancel(t.Context())
			wantErr := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
				wantErr = context.DeadlineExceeded
			}
			defer cancel()
			p := NewProxy("forward", listener.Addr().String(), transport.TLS)
			p.SetTLSConfig(&tls.Config{})
			msg := new(dns.Msg)
			msg.SetQuestion("example.org.", dns.TypeA)
			originalID := msg.Id
			result := make(chan error, 1)
			go func() {
				_, _, _, err := p.Connect(ctx, request.Request{Req: msg, W: &test.ResponseWriter{}}, Options{})
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("TLS client did not start the handshake")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, wantErr) {
					t.Errorf("Connect error = %v, want %v", err, wantErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("TLS connect did not finish")
			}
			if err := <-closed; err != nil {
				t.Errorf("stalled connection was not closed by the client: %v", err)
			}
			if msg.Id != originalID {
				t.Errorf("request ID = %d, want %d", msg.Id, originalID)
			}
			for _, conns := range p.transport.conns {
				if len(conns) != 0 {
					t.Fatal("failed TLS connection was cached")
				}
			}
		})
	}
}

func TestDialContextCanceledKeepsCachedConnection(t *testing.T) {
	server := dnstest.NewServer(func(_ dns.ResponseWriter, _ *dns.Msg) {})
	t.Cleanup(server.Close)
	for _, proto := range []string{"udp", "tcp"} {
		t.Run(proto, func(t *testing.T) {
			tr := newTransport("forward", server.Addr)
			pc, _, err := tr.Dial(proto)
			if err != nil {
				t.Fatal(err)
			}
			defer pc.c.Close()
			tr.Yield(pc)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, cached, err := tr.DialContext(ctx, proto); !errors.Is(err, context.Canceled) || cached {
				t.Fatalf("canceled DialContext: cached = %v, error = %v", cached, err)
			}
			reused, cached, err := tr.DialContext(t.Context(), proto)
			if err != nil || !cached || reused != pc {
				t.Fatalf("cached connection was lost: cached = %v, error = %v", cached, err)
			}
		})
	}
}

func TestConnectDNSIgnoresTLSConnectDeadline(t *testing.T) {
	server := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)
		response.SetReply(r)
		response.Answer = []dns.RR{test.A("example.org. 60 IN A 192.0.2.53")}
		w.WriteMsg(response)
	})
	t.Cleanup(server.Close)
	for _, tcp := range []bool{false, true} {
		name := "udp"
		if tcp {
			name = "tcp"
		}
		t.Run(name, func(t *testing.T) {
			p := NewProxy("forward", server.Addr, transport.DNS)
			t.Cleanup(func() { p.transport.cleanup(true) })
			msg := new(dns.Msg)
			msg.SetQuestion("example.org.", dns.TypeA)
			opts := Options{TLSConnectDeadline: time.Now().Add(-time.Second)}
			response, _, proto, err := p.Connect(t.Context(), request.Request{
				Req: msg, W: &test.ResponseWriter{TCP: tcp},
			}, opts)
			if err != nil {
				t.Fatalf("plain DNS was limited by the TLS deadline: %v", err)
			}
			if proto != name || len(response.Answer) != 1 {
				t.Fatalf("unexpected %s response: %v", proto, response)
			}
		})
	}
}
