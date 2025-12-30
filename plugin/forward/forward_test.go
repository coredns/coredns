package forward

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"

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

func (m *mockResponseWriter) LocalAddr() net.Addr         { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr        { return nil }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error { return nil }
func (m *mockResponseWriter) Write([]byte) (int, error)   { return 0, nil }
func (m *mockResponseWriter) Close() error                { return nil }
func (m *mockResponseWriter) TsigStatus() error           { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)         {}
func (m *mockResponseWriter) Hijack()                     {}

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
