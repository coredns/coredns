package forward

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// stalledTLSListener accepts TCP but withholds the TLS handshake on the first
// connections. Later connections are served normally by dns.Server.
type stalledTLSListener struct {
	net.Listener
	config        *tls.Config
	stalls        int64
	accepted      atomic.Int64
	firstAccepted chan struct{}
	closed        chan error
	wg            sync.WaitGroup
}

func (l *stalledTLSListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		n := l.accepted.Add(1)
		if n == 1 {
			close(l.firstAccepted)
		}
		if n > l.stalls {
			return tls.Server(conn, l.config), nil
		}
		l.wg.Go(func() {
			defer conn.Close()
			// Release an unbounded old client after the forwarding window has elapsed.
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, err := io.Copy(io.Discard, conn)
			l.closed <- err
		})
	}
}

func TestForwardTLSHandshakeRetry(t *testing.T) {
	originalTimeout := defaultTimeout
	defaultTimeout = 800 * time.Millisecond
	t.Cleanup(func() { defaultTimeout = originalTimeout })

	for _, tc := range []struct {
		name      string
		setting   string
		stalls    int64
		wantConns int64
		wantError bool
		tcp       bool
		deadline  time.Duration
		cancel    bool
		health    bool
	}{
		{name: "default", stalls: 1, wantConns: 2, health: true},
		{name: "TCP downstream", stalls: 1, wantConns: 2, tcp: true},
		{name: "explicit two attempts", setting: "max_connect_attempts 2", stalls: 1, wantConns: 2},
		{name: "unlimited attempts", setting: "max_connect_attempts 0", stalls: 1, wantConns: 2},
		{name: "single attempt", setting: "max_connect_attempts 1", stalls: 1, wantConns: 1, wantError: true},
		{name: "all handshakes stall", stalls: 2, wantConns: 2, wantError: true},
		{name: "caller deadline", stalls: 1, wantConns: 2, deadline: 500 * time.Millisecond},
		{name: "caller canceled", stalls: 1, wantConns: 1, wantError: true, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, roots := makeForwardDoQTestTLS(t)
			serverTLS.NextProtos = nil
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			upstream := &stalledTLSListener{
				Listener: listener, config: serverTLS, stalls: tc.stalls,
				closed: make(chan error, tc.stalls), firstAccepted: make(chan struct{}),
			}
			started := make(chan struct{})
			server := &dns.Server{
				Listener: upstream,
				Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
					response := new(dns.Msg)
					response.SetReply(r)
					response.Answer = []dns.RR{test.A("example.org. 60 IN A 192.0.2.53")}
					w.WriteMsg(response)
				}),
				NotifyStartedFunc: func() { close(started) },
			}
			serverDone := make(chan error, 1)
			go func() { serverDone <- server.ActivateAndServe() }()
			<-started
			t.Cleanup(func() {
				server.Shutdown()
				if err := <-serverDone; err != nil {
					t.Error(err)
				}
				upstream.wg.Wait()
			})

			controller := caddy.NewTestController("dns", fmt.Sprintf(`forward . tls://%s {
				tls_servername doq.test
				max_fails 0
				%s
			}`, listener.Addr(), tc.setting))
			fs, err := parseForward(controller)
			if err != nil {
				t.Fatal(err)
			}
			f := fs[0]
			p := f.proxies[0]
			config := p.GetTransport().GetTLSConfig().Clone()
			config.RootCAs = roots
			p.SetTLSConfig(config)
			// Manage the cache explicitly without background health-check connections.
			runtime.SetFinalizer(p, nil)
			p.GetTransport().Start()
			t.Cleanup(p.GetTransport().Stop)

			ctx := t.Context()
			if tc.deadline != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.deadline)
				defer cancel()
			} else if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				go func() {
					select {
					case <-upstream.firstAccepted:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			msg := new(dns.Msg)
			msg.SetQuestion("example.org.", dns.TypeA)
			msg.Id = 1234
			recorder := dnstest.NewRecorder(&test.ResponseWriter{TCP: tc.tcp})
			rcode, err := f.ServeDNS(ctx, recorder, msg)
			if tc.wantError {
				if err == nil || rcode != dns.RcodeServerFailure {
					t.Fatalf("rcode = %d, error = %v, want SERVFAIL and an error", rcode, err)
				}
			} else {
				if err != nil || rcode != 0 {
					t.Fatalf("rcode = %d, error = %v, want a successful retry", rcode, err)
				}
				if recorder.Msg == nil || recorder.Msg.Id != 1234 || len(recorder.Msg.Answer) != 1 ||
					recorder.Msg.Answer[0].String() != "example.org.\t60\tIN\tA\t192.0.2.53" {
					t.Fatalf("unexpected response: %v", recorder.Msg)
				}
				// A successful TLS connection must survive cancellation of its dial context.
				if _, err := f.ServeDNS(t.Context(), recorder, msg); err != nil {
					t.Fatalf("cached TLS connection failed: %v", err)
				}
			}
			if got := upstream.accepted.Load(); got != tc.wantConns {
				t.Errorf("accepted %d connections, want %d", got, tc.wantConns)
			}
			if msg.Id != 1234 {
				t.Errorf("request ID = %d, want 1234", msg.Id)
			}
			for range min(tc.stalls, upstream.accepted.Load()) {
				select {
				case err := <-upstream.closed:
					if err != nil {
						t.Errorf("stalled connection was not closed by the client: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("stalled connection was not closed")
				}
			}
			if tc.health {
				if err := p.GetHealthchecker().Check(p); err != nil {
					t.Fatalf("TLS health check failed after recovery: %v", err)
				}
				if p.Fails() != 0 || upstream.accepted.Load() != tc.wantConns+1 {
					t.Fatal("health check did not use a fresh, successful TLS connection")
				}
			}
		})
	}
}
