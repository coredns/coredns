package whoareyou

import (
	"net"
	"testing"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/pkg/dnsrecorder"
	"github.com/miekg/coredns/middleware/test"
	"github.com/miekg/coredns/middleware/whoareyou/ipscope"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func mustResolveUDPAddr(addr string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	return a
}

func TestWhoareyou(t *testing.T) {
	tests := []struct {
		next          middleware.Handler
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply []string
		expectedErr   error
		remoteAddr    net.Addr
	}{
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"127.0.0.1"},
			remoteAddr:    mustResolveUDPAddr("127.3.2.1:4321"),
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org",
			qtype:         dns.TypeAAAA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"::1"},
			remoteAddr:    mustResolveUDPAddr("[0:0:0:0:0:0:0:1]:5678"),
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.0.1", "192.168.0.2"},
			remoteAddr:    mustResolveUDPAddr("192.168.1.1:4321"),
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org",
			qtype:         dns.TypeAAAA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"fe80::d39:ec4c:ccb3:7a31"},
			remoteAddr:    mustResolveUDPAddr("[fe80::42:82ff:fe4b:5e97]:5678"),
		},
	}
	var ipset ipscope.IPSet
	for _, tc := range tests {
		for _, expected := range tc.expectedReply {
			ipset = append(ipset, net.ParseIP(expected))
		}
	}
	scopes := ipscope.NewIPScopes(ipset)
	wh := Whoareyou{scopes: scopes}

	ctx := context.TODO()

	for i, tc := range tests {
		wh.Next = tc.next
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

		rec := dnsrecorder.New(&responseWriter{tc.remoteAddr})
		code, err := wh.ServeDNS(ctx, rec, req)

		t.Logf("%s\n", rec.Msg)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v", i, tc.expectedErr, err)
		}
		if code != int(tc.expectedCode) {
			t.Errorf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
		}
		if len(tc.expectedReply) != 0 {
			for i, expected := range tc.expectedReply {
				var actual net.IP
				answer := rec.Msg.Answer[i]
				switch tc.qtype {
				case dns.TypeA:
					actual = answer.(*dns.A).A
				case dns.TypeAAAA:
					actual = answer.(*dns.AAAA).AAAA
				}
				if !actual.Equal(net.ParseIP(expected)) {
					t.Errorf("Test %d: Expected answer %s, but got %s", i, expected, actual.String())
				}
			}
		}
	}
}

type responseWriter struct {
	remoteAddr net.Addr
}

func (t *responseWriter) LocalAddr() net.Addr {
	ip := net.ParseIP("127.0.0.1")
	port := 53
	return &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

func (t *responseWriter) RemoteAddr() net.Addr {
	return t.remoteAddr
}

func (t *responseWriter) WriteMsg(m *dns.Msg) error     { return nil }
func (t *responseWriter) Write(buf []byte) (int, error) { return len(buf), nil }
func (t *responseWriter) Close() error                  { return nil }
func (t *responseWriter) TsigStatus() error             { return nil }
func (t *responseWriter) TsigTimersOnly(bool)           { return }
func (t *responseWriter) Hijack()                       { return }
