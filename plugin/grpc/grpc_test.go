package grpc

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/up"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	grpcgo "google.golang.org/grpc"
)

// newTestProxy creates a Proxy with a mock client for testing.
func newTestProxy(addr string, client pb.DnsServiceClient) *Proxy {
	p := &Proxy{
		addr:     addr,
		probe:    up.New(),
		maxFails: 2,
	}
	// Create a mock transport that returns the mock client
	p.transport = &grpcTransport{
		addr:      addr,
		poolSize:  1,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		proxyName: "grpc",
	}
	// Initialize with mock connection
	p.transport.connList.Store(&connList{conns: []*grpcConn{{client: client}}})
	// Start the transport's health checker
	p.transport.Start()
	return p
}

func TestGRPC(t *testing.T) {
	m := &dns.Msg{}
	msg, err := m.Pack()
	if err != nil {
		t.Fatalf("Error packing response: %s", err.Error())
	}
	dnsPacket := &pb.DnsPacket{Msg: msg}
	tests := map[string]struct {
		proxies []*Proxy
		wantErr bool
	}{
		"single_proxy_ok": {
			proxies: []*Proxy{
				newTestProxy("test1", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
			},
			wantErr: false,
		},
		"multiple_proxies_ok": {
			proxies: []*Proxy{
				newTestProxy("test1", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
				newTestProxy("test2", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
				newTestProxy("test3", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
			},
			wantErr: false,
		},
		"single_proxy_ko": {
			proxies: []*Proxy{
				newTestProxy("test1", &testServiceClient{dnsPacket: nil, err: errors.New("")}),
			},
			wantErr: true,
		},
		"multiple_proxies_one_ko": {
			proxies: []*Proxy{
				newTestProxy("test1", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
				newTestProxy("test2", &testServiceClient{dnsPacket: nil, err: errors.New("")}),
				newTestProxy("test3", &testServiceClient{dnsPacket: dnsPacket, err: nil}),
			},
			wantErr: false,
		},
		"multiple_proxies_ko": {
			proxies: []*Proxy{
				newTestProxy("test1", &testServiceClient{dnsPacket: nil, err: errors.New("")}),
				newTestProxy("test2", &testServiceClient{dnsPacket: nil, err: errors.New("")}),
				newTestProxy("test3", &testServiceClient{dnsPacket: nil, err: errors.New("")}),
			},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			g := newGRPC()
			g.from = "."
			g.proxies = tt.proxies
			// Cleanup transports after test
			defer func() {
				for _, p := range g.proxies {
					if p.transport != nil {
						p.transport.Stop()
					}
				}
			}()
			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			if _, err := g.ServeDNS(context.TODO(), rec, m); err != nil && !tt.wantErr {
				t.Fatal("Expected to receive reply, but didn't")
			}
		})
	}
}

// Test that fallthrough works correctly when there's no next plugin
func TestGRPCFallthroughNoNext(t *testing.T) {
	g := newGRPC()     // Use the constructor to properly initialize
	g.Fall = fall.Root // Enable fallthrough for all zones
	g.Next = nil       // No next plugin
	g.from = "."

	// Create a test request
	r := new(dns.Msg)
	r.SetQuestion("test.example.org.", dns.TypeA)

	w := &test.ResponseWriter{}

	// Should return SERVFAIL since no backends are configured and no next plugin
	rcode, err := g.ServeDNS(context.Background(), w, r)

	// Should not return the "no next plugin found" error
	if err != nil && strings.Contains(err.Error(), "no next plugin found") {
		t.Errorf("Expected no 'no next plugin found' error, got: %v", err)
	}

	// Should return SERVFAIL
	if rcode != dns.RcodeServerFailure {
		t.Errorf("Expected SERVFAIL when no backends and no next plugin, got: %d", rcode)
	}
}

// deadlineCheckingClient records whether a deadline was attached to ctx.
type deadlineCheckingClient struct {
	sawDeadline  atomic.Bool
	lastDeadline atomic.Pointer[time.Time]
	dnsPacket    *pb.DnsPacket
	err          error
}

func (c *deadlineCheckingClient) Query(ctx context.Context, in *pb.DnsPacket, opts ...grpcgo.CallOption) (*pb.DnsPacket, error) {
	if dl, ok := ctx.Deadline(); ok {
		c.sawDeadline.Store(true)
		c.lastDeadline.Store(&dl)
	}
	return c.dnsPacket, c.err
}

// Test that on error paths we still finish child spans, and that we set a per-call deadline.
func TestGRPC_SpansOnErrorPath(t *testing.T) {
	m := &dns.Msg{}
	msgBytes, err := m.Pack()
	if err != nil {
		t.Fatalf("Error packing response: %s", err)
	}
	dnsPacket := &pb.DnsPacket{Msg: msgBytes}

	// Proxy 1: returns error, we should still finish its child span and have a deadline
	p1 := &deadlineCheckingClient{dnsPacket: nil, err: errors.New("kaboom")}
	// Proxy 2: returns success
	p2 := &deadlineCheckingClient{dnsPacket: dnsPacket, err: nil}

	g := newGRPC()
	g.from = "."
	g.proxies = []*Proxy{newTestProxy("test1", p1), newTestProxy("test2", p2)}
	defer func() {
		for _, p := range g.proxies {
			if p.transport != nil {
				p.transport.Stop()
			}
		}
	}()

	// Ensure deterministic order of the retries: try p1 then p2
	g.p = new(sequential)

	// Set a parent span in context so ServeDNS creates child spans per attempt
	tracer := mocktracer.New()
	prev := ot.GlobalTracer()
	ot.SetGlobalTracer(tracer)
	defer ot.SetGlobalTracer(prev)

	parent := tracer.StartSpan("parent")
	ctx := ot.ContextWithSpan(t.Context(), parent)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := g.ServeDNS(ctx, rec, m); err != nil {
		t.Fatalf("ServeDNS returned error: %v", err)
	}

	// Assert both attempts finished child spans with retries
	// (2 query spans: error + success)
	finished := tracer.FinishedSpans()
	var finishedQueries int
	for _, s := range finished {
		if s.OperationName == "query" {
			finishedQueries++
		}
	}
	if finishedQueries != 2 {
		t.Fatalf("expected 2 finished 'query' spans, got %d (finished: %v)", finishedQueries, finished)
	}

	// Assert we set a deadline on the call contexts
	if !p1.sawDeadline.Load() {
		t.Fatalf("expected deadline to be set on first proxy call context")
	}
	if !p2.sawDeadline.Load() {
		t.Fatalf("expected deadline to be set on second proxy call context")
	}
}
