package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"net"
	"runtime"
	"strings"
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

	resp, _, _, err := p.Connect(context.Background(), req, Options{PreferUDP: true})
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

	_, _, _, err := p.Connect(context.Background(), req, Options{})
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

			resp, _, proto, err := p.Connect(context.Background(), req, tc.opts)
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

			if proto != tc.expectedProto {
				t.Errorf("Expected Connect to report proto %q, got %q", tc.expectedProto, proto)
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

		response, _, _, err := p.Connect(context.Background(), request, options)
		if err != nil {
			t.Errorf("Failed to connect to testdnsserver: %s", err)
			return
		}

		if response.Truncated != expectTruncated {
			t.Errorf("Expected truncated response for %s, but got TC flag %v", proto, response.Truncated)
		}
	}

	// Oversized UDP replies are truncated on Unix; Windows surfaces WSAEMSGSIZE instead.
	if runtime.GOOS != "windows" {
		// Test PreferUDP, expect truncated response
		testConnection("PreferUDP", Options{PreferUDP: true}, true)

		// Test No options specified, expect truncated response
		testConnection("NoOptionsSpecified", Options{}, true)
	}

	// Test ForceTCP, expect no truncated response
	testConnection("ForceTCP", Options{ForceTCP: true}, false)

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

func TestProxyMalformedUDPThenValid(t *testing.T) {
	tests := []struct {
		name      string
		malformed func(*dns.Msg) []byte
	}{
		{
			name: "short packet without complete header",
			malformed: func(r *dns.Msg) []byte {
				badID := r.Id + 1

				return []byte{
					byte(badID >> 8),
					byte(badID),
					0x00,
				}
			},
		},
		{
			name: "partial malformed response with mismatched ID",
			malformed: func(r *dns.Msg) []byte {
				badID := r.Id + 1

				// Complete DNS header claiming one question, followed by an
				// incomplete one-byte label. ReadMsg returns a partial message
				// containing badID together with an unpacking error.
				return []byte{
					byte(badID >> 8),
					byte(badID),
					0x81,
					0x80,
					0x00,
					0x01,
					0x00,
					0x00,
					0x00,
					0x00,
					0x00,
					0x00,
					0x01,
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serverResult := make(chan error, 1)

			s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
				if _, err := w.Write(tc.malformed(r)); err != nil {
					serverResult <- err
					return
				}

				// Immediately send a completely valid response for the same query.
				reply := new(dns.Msg)
				reply.SetReply(r)
				reply.Answer = append(
					reply.Answer,
					test.A("variant.example. IN A 192.0.2.55"),
				)

				if err := w.WriteMsg(reply); err != nil {
					serverResult <- err
					return
				}

				serverResult <- nil
			})
			defer s.Close()

			p := NewProxy(
				"TestProxyMalformedUDPThenValid",
				s.Addr,
				transport.DNS,
			)
			p.readTimeout = time.Second
			p.Start(5 * time.Second)
			defer p.Stop()

			q := new(dns.Msg)
			q.SetQuestion("variant.example.", dns.TypeA)

			req := request.Request{
				Req: q,
				W:   &test.ResponseWriter{},
			}

			resp, _, _, err := p.Connect(
				context.Background(),
				req,
				Options{PreferUDP: true},
			)

			if serverErr := <-serverResult; serverErr != nil {
				t.Fatalf("upstream failed to send responses: %v", serverErr)
			}

			// Expected: ignore the malformed UDP datagram and read the valid response.
			if err != nil {
				t.Fatalf(
					"valid response after malformed UDP datagram was not accepted: %v",
					err,
				)
			}

			if resp == nil {
				t.Fatal("expected valid response, got nil")
			}

			if len(resp.Answer) != 1 {
				t.Fatalf("expected one answer, got %d", len(resp.Answer))
			}

			if got := resp.Answer[0].String(); !strings.Contains(got, "192.0.2.55") {
				t.Fatalf("expected 192.0.2.55, got %q", got)
			}
		})
	}
}
