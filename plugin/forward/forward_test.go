package forward

import (
	"context"
	"fmt"
	"net"
	"runtime"
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
// maxConnectAttempts is configured, and that an explicit maxConnectAttempts=0
// keeps the legacy behaviour (no per-request cap).
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
			f.maxConnectAttemptsSet = true

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

func TestForward_DefaultConnectAttemptCap(t *testing.T) {
	for _, proxyCount := range []int{1, 3} {
		t.Run(fmt.Sprintf("%d upstreams", proxyCount), func(t *testing.T) {
			f := New()
			f.opts.ForceTCP = true
			f.maxfails = 0

			listeners := make([]net.Listener, 0, proxyCount)
			t.Cleanup(func() {
				for _, listener := range listeners {
					_ = listener.Close()
				}
			})
			for range proxyCount {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("failed to allocate upstream address: %v", err)
				}
				listeners = append(listeners, listener)
			}

			upstreams := make([]string, 0, proxyCount)
			for _, listener := range listeners {
				upstream := listener.Addr().String()
				if err := listener.Close(); err != nil {
					t.Fatalf("failed to close upstream listener: %v", err)
				}
				upstreams = append(upstreams, upstream)
				f.SetProxy(proxy.NewProxy("forward", upstream, "tcp"))
			}

			tracer := mocktracer.New()
			span := tracer.StartSpan("test")
			ctx := opentracing.ContextWithSpan(context.Background(), span)
			ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)

			_, err := f.ServeDNS(ctx, &mockResponseWriter{}, req)
			if err == nil {
				t.Fatal("expected connection refused error")
			}

			want := defaultConnectAttemptsPerUpstream * proxyCount
			spans := tracer.FinishedSpans()
			if got := len(spans); got != want {
				t.Fatalf("expected %d connect attempts, got %d", want, got)
			}

			attemptsByUpstream := make(map[string]int, proxyCount)
			for _, span := range spans {
				upstream, ok := span.Tags()["peer.address"].(string)
				if !ok {
					t.Fatal("connect attempt is missing peer.address")
				}
				attemptsByUpstream[upstream]++
			}
			for _, upstream := range upstreams {
				if got := attemptsByUpstream[upstream]; got != defaultConnectAttemptsPerUpstream {
					t.Errorf("expected %d attempts to %s, got %d", defaultConnectAttemptsPerUpstream, upstream, got)
				}
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

// TestForwardTruncatedNodata distinguishes an incomplete reply from a complete
// empty answer, including replies truncated by the proxy after a UDP overflow.
func TestForwardTruncatedNodata(t *testing.T) {
	tests := []struct {
		name          string
		options       string
		tcpClient     bool
		overflow      bool
		tcpTruncated  bool
		tcpAnswer     bool
		rcode         int
		wantUDP       int32
		wantTCP       int32
		wantNext      int32
		wantTruncated bool
		wantAnswers   int
	}{
		{name: "udp", wantUDP: 1, wantTruncated: true},
		{name: "tcp", tcpClient: true, tcpTruncated: true, wantTCP: 1, wantTruncated: true},
		{name: "udpOverflow", overflow: true, wantUDP: 1, wantTruncated: true},
		{name: "preferUDPAnswer", options: "prefer_udp", tcpClient: true, tcpAnswer: true, wantUDP: 1, wantTCP: 1, wantAnswers: 1},
		{name: "preferUDPNodata", options: "prefer_udp", wantUDP: 1, wantTCP: 1, wantNext: 1, wantAnswers: 1},
		{name: "preferUDPTruncated", options: "prefer_udp", tcpTruncated: true, wantUDP: 1, wantTCP: 1, wantTruncated: true},
		{name: "preferUDPOverflow", options: "prefer_udp", overflow: true, tcpAnswer: true, wantUDP: 1, wantTCP: 1, wantAnswers: 1},
		{name: "forceTCP", options: "force_tcp", tcpTruncated: true, wantTCP: 1, wantTruncated: true},
		{name: "forceTCPOverPreferUDP", options: "force_tcp\nprefer_udp", tcpTruncated: true, wantTCP: 1, wantTruncated: true},
		{name: "nextNOERROR", options: "next NOERROR", wantUDP: 1, wantNext: 1, wantAnswers: 1},
		{name: "unmatchedNXDOMAIN", rcode: dns.RcodeNameError, wantUDP: 1, wantTruncated: true},
		{name: "nextNXDOMAIN", options: "next NXDOMAIN", rcode: dns.RcodeNameError, wantUDP: 1, wantNext: 1, wantAnswers: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.overflow && runtime.GOOS == "windows" {
				t.Skip("oversized UDP replies return WSAEMSGSIZE on Windows")
			}
			var udp, tcp, next atomic.Int32
			s1 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
				ret := new(dns.Msg)
				ret.SetRcode(r, tc.rcode)
				if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
					udp.Add(1)
					ret.Truncated = !tc.overflow
					if tc.overflow {
						// Send more than the client's 512-byte buffer without TC.
						for range 40 {
							ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
						}
					}
				} else {
					tcp.Add(1)
					ret.Truncated = tc.tcpTruncated
					if tc.tcpAnswer {
						ret.Answer = []dns.RR{test.A("example.org. IN A 127.0.0.1")}
					}
				}
				w.WriteMsg(ret)
			})
			t.Cleanup(s1.Close)
			s2 := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
				next.Add(1)
				ret := new(dns.Msg)
				ret.SetReply(r)
				ret.Answer = []dns.RR{test.A("example.org. IN A 127.0.0.2")}
				w.WriteMsg(ret)
			})
			t.Cleanup(s2.Close)

			c := caddy.NewTestController("dns", fmt.Sprintf(`
				forward . %s {
					max_fails 0
					next_on_nodata
					%s
				}
				forward . %s {
					max_fails 0
				}`, s1.Addr, tc.options, s2.Addr))
			fs, err := parseForward(c)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range fs {
				if err := f.OnStartup(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { f.OnShutdown() })
			}
			f := fs[0]
			f.Next = fs[1]
			m := new(dns.Msg)
			m.SetQuestion("example.org.", dns.TypeA)
			id := m.Id
			rec := dnstest.NewRecorder(&test.ResponseWriter{TCP: tc.tcpClient})
			if code, err := f.ServeDNS(context.Background(), rec, m); code != 0 || err != nil {
				t.Fatalf("ServeDNS returned %d, %v; want 0, nil", code, err)
			}
			if udp.Load() != tc.wantUDP || tcp.Load() != tc.wantTCP || next.Load() != tc.wantNext {
				t.Errorf("got exchanges UDP=%d TCP=%d next=%d; want UDP=%d TCP=%d next=%d", udp.Load(), tcp.Load(), next.Load(), tc.wantUDP, tc.wantTCP, tc.wantNext)
			}
			if rec.Msg == nil {
				t.Fatal("no reply written")
			}
			wantRcode := tc.rcode
			if tc.wantNext != 0 {
				wantRcode = dns.RcodeSuccess
			}
			if rec.Msg.Truncated != tc.wantTruncated || len(rec.Msg.Answer) != tc.wantAnswers || rec.Msg.Rcode != wantRcode {
				t.Errorf("got reply TC=%t answers=%d rcode=%d; want TC=%t answers=%d rcode=%d", rec.Msg.Truncated, len(rec.Msg.Answer), rec.Msg.Rcode, tc.wantTruncated, tc.wantAnswers, wantRcode)
			}
			if m.Id != id || rec.Msg.Id != id {
				t.Errorf("got IDs request=%d reply=%d; want %d", m.Id, rec.Msg.Id, id)
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

func TestForwardDoesNotRetryLocalPackError(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	req.Answer = []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{
			Name:   "example.org.",
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
		},
		// A TXT character-string is limited to 255 wire bytes. This is a
		// deterministic local serialization error and is unrelated to HIP or
		// compression-pointer handling in miekg/dns.
		Txt: []string{strings.Repeat("x", 256)},
	}}

	f := New()
	f.maxfails = 0
	f.maxConnectAttempts = 2
	f.maxConnectAttemptsSet = true
	f.opts.ForceTCP = true
	f.proxies = []*proxy.Proxy{
		proxy.NewProxy("forward", "127.0.0.1:1", transport.DNS),
	}

	tracer := mocktracer.New()
	span := tracer.StartSpan("test")
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	rcode, err := f.ServeDNS(ctx, &mockResponseWriter{}, req)
	if rcode != dns.RcodeFormatError {
		t.Fatalf("expected FORMERR, got %s", dns.RcodeToString[rcode])
	}
	if err == nil || !strings.Contains(err.Error(), "string exceeded 255 bytes in txt") {
		t.Fatalf("expected local TXT packing error, got %v", err)
	}

	// ServeDNS starts one child span for each forwarding attempt. A local
	// packing failure must stop after the first attempt even though the normal
	// connect-attempt limit permits two attempts.
	if got := len(tracer.FinishedSpans()); got != 1 {
		t.Fatalf("expected one forwarding attempt, got %d", got)
	}
}
