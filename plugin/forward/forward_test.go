package forward

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*proxy.Proxy{
			proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
			proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
			proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
		},
		p: &roundRobin{},
	}

	expect := []*proxy.Proxy{
		proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
		proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
		proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
	}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.Addr() != expect[i].Addr() {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].Addr(), p.Addr())
		}
	}
}

func TestSetTapPlugin(t *testing.T) {
	input := `forward . 127.0.0.1
	dnstap /tmp/dnstap.sock full
	dnstap tcp://example.com:6000
	`
	stanzas := strings.Split(input, "\n")
	c := caddy.NewTestController("dns", strings.Join(stanzas[1:], "\n"))
	dnstapSetup, err := caddy.DirectiveAction("dns", "dnstap")
	if err != nil {
		t.Fatal(err)
	}
	if err = dnstapSetup(c); err != nil {
		t.Fatal(err)
	}
	c.Dispenser = caddyfile.NewDispenser("", strings.NewReader(stanzas[0]))
	if err = setup(c); err != nil {
		t.Fatal(err)
	}
	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})
	f, ok := dnsserver.GetConfig(c).Handler("forward").(*Forward)
	if !ok {
		t.Fatal("Expected a forward plugin")
	}
	tap, ok := dnsserver.GetConfig(c).Handler("dnstap").(*dnstap.Dnstap)
	if !ok {
		t.Fatal("Expected a dnstap plugin")
	}
	f.SetTapPlugin(tap)
	if len(f.tapPlugins) != 2 {
		t.Fatalf("Expected: 2 results, got: %v", len(f.tapPlugins))
	}
	if f.tapPlugins[0] != tap || tap.Next != f.tapPlugins[1] {
		t.Error("Unexpected order of dnstap plugins")
	}
}

type mockResponseWriter struct{}

func (m *mockResponseWriter) LocalAddr() net.Addr          { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr         { return nil }
func (m *mockResponseWriter) WriteMsg(_msg *dns.Msg) error { return nil }
func (m *mockResponseWriter) Write([]byte) (int, error)    { return 0, nil }
func (m *mockResponseWriter) Close() error                 { return nil }
func (m *mockResponseWriter) TsigStatus() error            { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)          {}
func (m *mockResponseWriter) Hijack()                      {}

// TestForward_Regression_NoBusyLoop ensures that ServeDNS does not perform
// an unbounded number of upstream connect attempts for a single request when
// maxConnectAttempts is configured, and that maxConnectAttempts=0 keeps the
// legacy behaviour (no per-request cap).
func TestForward_Regression_NoBusyLoop(t *testing.T) {
	tests := []struct {
		name        string
		maxAttempts uint32
	}{
		{name: "unbounded", maxAttempts: 0},
		{name: "single attempt", maxAttempts: 1},
		{name: "10 attempts", maxAttempts: 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := New()

			// ForceTCP ensures that connection refused errors happen immediately on Dial.
			f.opts.ForceTCP = true
			// Disable healthcheck so that only the per-request attempts cap applies here.
			f.maxfails = 0

			// Set maxConnectAttempts to the number of attempts we want to test.
			f.maxConnectAttempts = tc.maxAttempts

			// Assume nothing is listening on this port, so the connection will be refused.
			p := proxy.NewProxy("forward", "127.0.0.1:54321", "tcp")
			f.SetProxy(p)

			// Create a mock tracer to count the number of connection attempts.
			tracer := mocktracer.New()
			span := tracer.StartSpan("test")

			ctx := opentracing.ContextWithSpan(context.Background(), span)
			timeout := 500 * time.Millisecond
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)

			rw := &mockResponseWriter{}

			_, err := f.ServeDNS(ctx, rw, req)
			spans := tracer.FinishedSpans()

			if err == nil {
				t.Errorf("Expected error from ServeDNS due to connection refused, got nil")
			}

			// In all cases we expect at least one attempt/span.
			if len(spans) == 0 {
				t.Errorf("Expected at least 1 span, got 0")
			}

			// When maxConnectAttempts is configured (> 0), the number of connect
			// attempts as observed via spans should be equal to the configured value.
			if tc.maxAttempts > 0 && uint32(len(spans)) != tc.maxAttempts {
				t.Errorf("Expected %d spans, got %d", tc.maxAttempts, len(spans))
			}
		})
	}
}

func TestForward_NextOnNodata(t *testing.T) {
	tests := []struct {
		name         string
		nextOnNodata bool
	}{
		{name: "serveEmpty", nextOnNodata: false},
		{name: "nextNotEmpty", nextOnNodata: true},
	}

	s1 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	s2 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s1.Close()
	defer s2.Close()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var config string
			if tc.nextOnNodata {
				config = `
					forward . %s {
						next_on_nodata
					}
					forward . %s
					`
			} else {
				config = `
					forward . %s
					forward . %s
					`
			}
			c := caddy.NewTestController("dns", fmt.Sprintf(config, s1.Addr, s2.Addr))
			fs, err := parseForward(c)
			if err != nil {
				t.Errorf("Failed to create forwarder: %s", err)
			}
			if x := len(fs); x != 2 {
				t.Errorf("Failed to create two forward instances")
			}
			f := fs[0]
			f.Next = fs[1]
			f.OnStartup()
			defer f.OnShutdown()

			m := new(dns.Msg)
			m.SetQuestion("example.org.", dns.TypeA)
			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
				t.Fatal("Expected to receive reply, but didn't")
			}
			if x := rec.Rcode; x != dns.RcodeSuccess {
				t.Errorf("Expected %v, got %+v instead", dns.RcodeSuccess, rec)
			}
			if tc.nextOnNodata {
				if x := len(rec.Msg.Answer); x != 1 {
					t.Errorf("Expected answer, got %d instead", x)
				}
				if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
					t.Errorf("Expected %s, got %s", "example.org.", x)
				}
			} else {
				if x := len(rec.Msg.Answer); x != 0 {
					t.Errorf("Expected zero length answer, got %d instead", x)
				}
			}
		})
	}
}

func TestForwardFailoverStopsAfterAllUpstreams(t *testing.T) {
	var first atomic.Int32
	var second atomic.Int32

	s1 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		first.Add(1)

		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
	})
	defer s1.Close()

	s2 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		second.Add(1)

		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
	})
	defer s2.Close()

	c := caddy.NewTestController("dns", fmt.Sprintf(`forward . %s %s {
		policy sequential
		failover SERVFAIL
	}`, s1.Addr, s2.Addr))

	fs, err := parseForward(c)
	if err != nil {
		t.Fatal(err)
	}

	f := fs[0]
	if err := f.OnStartup(); err != nil {
		t.Fatal(err)
	}
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	ctx, cancel := context.WithTimeout(
		context.Background(),
		200*time.Millisecond,
	)
	defer cancel()

	if _, err := f.ServeDNS(ctx, rec, req); err != nil {
		t.Fatalf("Expected the last SERVFAIL response, got error: %v", err)
	}

	if rec.Rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected SERVFAIL, got %d", rec.Rcode)
	}

	if got := first.Load(); got != 1 {
		t.Errorf("Expected first upstream to be queried once, got %d", got)
	}
	if got := second.Load(); got != 1 {
		t.Errorf("Expected second upstream to be queried once, got %d", got)
	}
}
