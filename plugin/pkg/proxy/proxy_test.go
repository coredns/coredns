package proxy

import (
	"context"
	"crypto/tls"
	"math"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestProxy(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := NewProxy("TestProxy", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	p.Start(5 * time.Second)
	m := new(dns.Msg)

	m.SetQuestion("example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	req := request.Request{Req: m, W: rec}

	resp, err := p.Connect(context.Background(), req, Options{PreferUDP: true})
	if err != nil {
		t.Errorf("Failed to connect to testdnsserver: %s", err)
	}

	if x := resp.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}

func TestProxyTLSFail(t *testing.T) {
	// This is an udp/tcp test server, so we shouldn't reach it with TLS.
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := NewProxy("TestProxyTLSFail", s.Addr, transport.TLS)
	p.readTimeout = 10 * time.Millisecond
	p.SetTLSConfig(&tls.Config{})
	p.Start(5 * time.Second)
	m := new(dns.Msg)

	m.SetQuestion("example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	req := request.Request{Req: m, W: rec}

	_, err := p.Connect(context.Background(), req, Options{})
	if err == nil {
		t.Fatal("Expected *not* to receive reply, but got one")
	}
}

func TestProtocolSelection(t *testing.T) {
	p := NewProxy("TestProtocolSelection", "bad_address", transport.DNS)
	p.readTimeout = 10 * time.Millisecond

	stateUDP := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	stateTCP := request.Request{W: &test.ResponseWriter{TCP: true}, Req: new(dns.Msg)}
	ctx := context.TODO()

	go func() {
		p.Connect(ctx, stateUDP, Options{})
		p.Connect(ctx, stateUDP, Options{ForceTCP: true})
		p.Connect(ctx, stateUDP, Options{PreferUDP: true})
		p.Connect(ctx, stateUDP, Options{PreferUDP: true, ForceTCP: true})
		p.Connect(ctx, stateTCP, Options{})
		p.Connect(ctx, stateTCP, Options{ForceTCP: true})
		p.Connect(ctx, stateTCP, Options{PreferUDP: true})
		p.Connect(ctx, stateTCP, Options{PreferUDP: true, ForceTCP: true})
	}()

	for i, exp := range []string{"udp", "tcp", "udp", "tcp", "tcp", "tcp", "udp", "tcp"} {
		proto := <-p.transport.dial
		p.transport.ret <- nil
		if proto != exp {
			t.Errorf("Unexpected protocol in case %d, expected %q, actual %q", i, exp, proto)
		}
	}
}

func TestProxyIncrementFails(t *testing.T) {
	var testCases = []struct {
		name        string
		fails       uint32
		expectFails uint32
	}{
		{
			name:        "increment fails counter overflows",
			fails:       math.MaxUint32,
			expectFails: math.MaxUint32,
		},
		{
			name:        "increment fails counter",
			fails:       0,
			expectFails: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProxy("TestProxyIncrementFails", "bad_address", transport.DNS)
			p.fails = tc.fails
			p.incrementFails()
			if p.fails != tc.expectFails {
				t.Errorf("Expected fails to be %d, got %d", tc.expectFails, p.fails)
			}
		})
	}
}
