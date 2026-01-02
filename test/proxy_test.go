package test

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestLookupProxy(t *testing.T) {
	t.Parallel()
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	// expect answer section with A record in it
	if len(resp.Answer) == 0 {
		t.Fatalf("Expected to at least one RR in the answer section, got none: %s", resp)
	}
	if resp.Answer[0].Header().Rrtype != dns.TypeA {
		t.Errorf("Expected RR to A, got: %d", resp.Answer[0].Header().Rrtype)
	}
	if resp.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1, got: %s", resp.Answer[0].(*dns.A).A.String())
	}
}

func BenchmarkProxyLookup(b *testing.B) {
	t := new(testing.T)
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to created zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
		file ` + name + `
	}`

	i, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	udp, _ := CoreDNSServerPorts(i, 0)
	if udp == "" {
		t.Fatalf("Could not get udp listening port")
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)

	for b.Loop() {
		if _, err := dns.Exchange(m, udp); err != nil {
			b.Fatal("Expected to receive reply, but didn't")
		}
	}
}

// BenchmarkProxyWithMultipleBackends verifies the serialization issue by running concurrent load
// against 1, 2, and 3 backend proxies using the forward plugin.
func BenchmarkProxyWithMultipleBackends(b *testing.B) {
	// Start a dummy upstream server
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	counts := []int{1, 2, 3}

	for _, n := range counts {
		b.Run(fmt.Sprintf("%d-Backends", n), func(b *testing.B) {
			f := forward.New()
			f.SetProxyOptions(proxy.Options{PreferUDP: true})

			proxies := make([]*proxy.Proxy, n)
			for i := range n {
				p := proxy.NewProxy(fmt.Sprintf("proxy-%d", i), s.Addr, "dns")
				f.SetProxy(p)
				proxies[i] = p
			}
			defer func() {
				for _, p := range proxies {
					p.Stop()
				}
			}()

			// Pre-warm connections
			ctx := context.Background()
			m := new(dns.Msg)
			m.SetQuestion("example.org.", dns.TypeA)
			noop := &benchmarkResponseWriter{}

			for range n * 10 {
				f.ServeDNS(ctx, noop, m)
			}

			b.ResetTimer()
			b.ReportAllocs()

			b.RunParallel(func(pb *testing.PB) {
				m := new(dns.Msg)
				m.SetQuestion("example.org.", dns.TypeA)
				ctx := context.Background()
				w := &benchmarkResponseWriter{}

				for pb.Next() {
					// forward plugin handles selection via its policy (default random)
					f.ServeDNS(ctx, w, m)
				}
			})
		})
	}
}

type benchmarkResponseWriter struct{}

func (b *benchmarkResponseWriter) LocalAddr() net.Addr         { return nil }
func (b *benchmarkResponseWriter) RemoteAddr() net.Addr        { return nil }
func (b *benchmarkResponseWriter) WriteMsg(m *dns.Msg) error   { return nil }
func (b *benchmarkResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (b *benchmarkResponseWriter) Close() error                { return nil }
func (b *benchmarkResponseWriter) TsigStatus() error           { return nil }
func (b *benchmarkResponseWriter) TsigTimersOnly(bool)         {}
func (b *benchmarkResponseWriter) Hijack()                     {}
