package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"net"
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
	testCases := []struct {
		name          string
		requestTCP    bool // true = TCP request, false = UDP request
		opts          Options
		expectedProto string
	}{
		{"UDP request, no options", false, Options{}, "udp"},
		{"UDP request, ForceTCP", false, Options{ForceTCP: true}, "tcp"},
		{"UDP request, PreferUDP", false, Options{PreferUDP: true}, "udp"},
		{"UDP request, ForceTCP+PreferUDP", false, Options{ForceTCP: true, PreferUDP: true}, "tcp"},
		{"TCP request, no options", true, Options{}, "tcp"},
		{"TCP request, ForceTCP", true, Options{ForceTCP: true}, "tcp"},
		{"TCP request, PreferUDP", true, Options{PreferUDP: true}, "udp"},
		{"TCP request, ForceTCP+PreferUDP", true, Options{ForceTCP: true, PreferUDP: true}, "tcp"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Track which protocol the server received (use channel to avoid data race)
			protoChan := make(chan string, 1)
			s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
				// Determine protocol from the connection type
				if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
					protoChan <- "tcp"
				} else {
					protoChan <- "udp"
				}
				ret := new(dns.Msg)
				ret.SetReply(r)
				ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
				w.WriteMsg(ret)
			})
			defer s.Close()

			p := NewProxy("TestProtocolSelection", s.Addr, transport.DNS)
			p.readTimeout = 1 * time.Second
			p.Start(5 * time.Second)
			defer p.Stop()

			m := new(dns.Msg)
			m.SetQuestion("example.org.", dns.TypeA)

			req := request.Request{
				W:   &test.ResponseWriter{TCP: tc.requestTCP},
				Req: m,
			}

			resp, err := p.Connect(context.Background(), req, tc.opts)
			if err != nil {
				t.Fatalf("Connect failed: %v", err)
			}
			if resp == nil {
				t.Fatal("Expected response, got nil")
			}

			receivedProto := <-protoChan
			if receivedProto != tc.expectedProto {
				t.Errorf("Expected protocol %q, but server received %q", tc.expectedProto, receivedProto)
			}
		})
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

func TestCoreDNSOverflow(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)

		answers := []dns.RR{
			test.A("example.org. IN A 127.0.0.1"),
			test.A("example.org. IN A 127.0.0.2"),
			test.A("example.org. IN A 127.0.0.3"),
			test.A("example.org. IN A 127.0.0.4"),
			test.A("example.org. IN A 127.0.0.5"),
			test.A("example.org. IN A 127.0.0.6"),
			test.A("example.org. IN A 127.0.0.7"),
			test.A("example.org. IN A 127.0.0.8"),
			test.A("example.org. IN A 127.0.0.9"),
			test.A("example.org. IN A 127.0.0.10"),
			test.A("example.org. IN A 127.0.0.11"),
			test.A("example.org. IN A 127.0.0.12"),
			test.A("example.org. IN A 127.0.0.13"),
			test.A("example.org. IN A 127.0.0.14"),
			test.A("example.org. IN A 127.0.0.15"),
			test.A("example.org. IN A 127.0.0.16"),
			test.A("example.org. IN A 127.0.0.17"),
			test.A("example.org. IN A 127.0.0.18"),
			test.A("example.org. IN A 127.0.0.19"),
			test.A("example.org. IN A 127.0.0.20"),
		}
		ret.Answer = answers
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := NewProxy("TestCoreDNSOverflow", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	p.Start(5 * time.Second)
	defer p.Stop()

	// Test different connection modes
	testConnection := func(proto string, options Options, expectTruncated bool) {
		t.Helper()

		queryMsg := new(dns.Msg)
		queryMsg.SetQuestion("example.org.", dns.TypeA)

		recorder := dnstest.NewRecorder(&test.ResponseWriter{})
		request := request.Request{Req: queryMsg, W: recorder}

		response, err := p.Connect(context.Background(), request, options)
		if err != nil {
			t.Errorf("Failed to connect to testdnsserver: %s", err)
		}

		if response.Truncated != expectTruncated {
			t.Errorf("Expected truncated response for %s, but got TC flag %v", proto, response.Truncated)
		}
	}

	// Test PreferUDP, expect truncated response
	testConnection("PreferUDP", Options{PreferUDP: true}, true)

	// Test ForceTCP, expect no truncated response
	testConnection("ForceTCP", Options{ForceTCP: true}, false)

	// Test No options specified, expect truncated response
	testConnection("NoOptionsSpecified", Options{}, true)

	// Test both TCP and UDP provided, expect no truncated response
	testConnection("BothTCPAndUDP", Options{PreferUDP: true, ForceTCP: true}, false)
}

func TestShouldTruncateResponse(t *testing.T) {
	testCases := []struct {
		testname string
		err      error
		expected bool
	}{
		{"BadAlgorithm", dns.ErrAlg, false},
		{"BufferSizeTooSmall", dns.ErrBuf, true},
		{"OverflowUnpackingA", errors.New("overflow unpacking a"), true},
		{"OverflowingHeaderSize", errors.New("overflowing header size"), true},
		{"OverflowpackingA", errors.New("overflow packing a"), true},
		{"ErrSig", dns.ErrSig, false},
	}

	for _, tc := range testCases {
		t.Run(tc.testname, func(t *testing.T) {
			result := shouldTruncateResponse(tc.err)
			if result != tc.expected {
				t.Errorf("For testname '%v', expected %v but got %v", tc.testname, tc.expected, result)
			}
		})
	}
}
