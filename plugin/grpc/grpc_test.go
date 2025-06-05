package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

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
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
			},
			wantErr: false,
		},
		"multiple_proxies_ok": {
			proxies: []*Proxy{
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
			},
			wantErr: false,
		},
		"single_proxy_ko": {
			proxies: []*Proxy{
				{client: &testServiceClient{dnsPacket: nil, err: errors.New("")}},
			},
			wantErr: true,
		},
		"multiple_proxies_one_ko": {
			proxies: []*Proxy{
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
				{client: &testServiceClient{dnsPacket: nil, err: errors.New("")}},
				{client: &testServiceClient{dnsPacket: dnsPacket, err: nil}},
			},
			wantErr: false,
		},
		"multiple_proxies_ko": {
			proxies: []*Proxy{
				{client: &testServiceClient{dnsPacket: nil, err: errors.New("")}},
				{client: &testServiceClient{dnsPacket: nil, err: errors.New("")}},
				{client: &testServiceClient{dnsPacket: nil, err: errors.New("")}},
			},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			g := newGRPC()
			g.from = "."
			g.proxies = tt.proxies
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
