package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"strconv"
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

func TestCoreDNSOverflowWithoutEdns(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		answers := []dns.RR{
			test.A("example123.org. IN A 127.0.0.1"),
			test.A("example123.org. IN A 127.0.0.2"),
			test.A("example123.org. IN A 127.0.0.3"),
			test.A("example123.org. IN A 127.0.0.4"),
			test.A("example123.org. IN A 127.0.0.5"),
			test.A("example123.org. IN A 127.0.0.6"),
			test.A("example123.org. IN A 127.0.0.7"),
			test.A("example123.org. IN A 127.0.0.8"),
			test.A("example123.org. IN A 127.0.0.9"),
			test.A("example123.org. IN A 127.0.0.10"),
			test.A("example123.org. IN A 127.0.0.11"),
			test.A("example123.org. IN A 127.0.0.12"),
			test.A("example123.org. IN A 127.0.0.13"),
			test.A("example123.org. IN A 127.0.0.14"),
			test.A("example123.org. IN A 127.0.0.15"),
			test.A("example123.org. IN A 127.0.0.16"),
			test.A("example123.org. IN A 127.0.0.17"),
			test.A("example123.org. IN A 127.0.0.18"),
			test.A("example123.org. IN A 127.0.0.19"),
			test.A("example123.org. IN A 127.0.0.20"),
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
		queryMsg.SetQuestion("example123.org.", dns.TypeA)

		recorder := dnstest.NewRecorder(&test.ResponseWriter{})
		request := request.Request{Req: queryMsg, W: recorder}
		response, err := p.Connect(context.Background(), request, options)

		if err != nil {
			t.Errorf("Failed to connect to testdnsserver: %s", err)
		}
		if response.Rcode != dns.RcodeSuccess {
			t.Errorf("Expected No error, but got %v", response.Rcode)
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

func TestCoreDNSOverflowForDifferentRanges(t *testing.T) {
	// Testing with 19 characters in the domain name.
	// -----------------------------------------------------------------------------------------------------------
	// Length of the response for 12 A records will be 478 bytes and UDPSize is 512.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "testingexample.org.", 12, 512, "PreferUDP", false)
	testCoreDNSOverflow(t, "testingexample.org.", 12, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "testingexample.org.", 12, 512, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "testingexample.org.", 12, 512, "BothTCPAndUDP", false)

	// Length of the response for 13 A records will be 512 bytes and UDPSize is 512.
	// This is a case where the response len is 512 bytes and protocol will be switched to TCP.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "testingexample.org.", 13, 512, "PreferUDP", false)
	testCoreDNSOverflow(t, "testingexample.org.", 13, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "testingexample.org.", 13, 512, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "testingexample.org.", 13, 512, "BothTCPAndUDP", false)

	// Length of the response for 14 A records will be 546 bytes and UDPSize is 512.
	// Overflow error will be occur and truncated response will be returned with TC bit set for UDP protocol.
	testCoreDNSOverflow(t, "testingexample.org.", 14, 512, "PreferUDP", true)
	testCoreDNSOverflow(t, "testingexample.org.", 14, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "testingexample.org.", 14, 512, "NoOptionsSpecified", true)
	testCoreDNSOverflow(t, "testingexample.org.", 14, 512, "BothTCPAndUDP", false)

	// Testing with 12 characters in the domain name
	// -----------------------------------------------------------------------------------------------------------
	// Length of the response for 16 A records will be 488 bytes and UDPSize is 515.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example.org.", 16, 515, "PreferUDP", false)
	testCoreDNSOverflow(t, "example.org.", 16, 515, "ForceTCP", false)
	testCoreDNSOverflow(t, "example.org.", 16, 515, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example.org.", 16, 515, "BothTCPAndUDP", false)

	// Length of the response for 17 A records will be 515 bytes and UDPSize is 515.
	// This is a case where the response len is 515 bytes and protocol will be switched to TCP.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example.org.", 17, 515, "PreferUDP", false)
	testCoreDNSOverflow(t, "example.org.", 17, 515, "ForceTCP", false)
	testCoreDNSOverflow(t, "example.org.", 17, 515, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example.org.", 17, 515, "BothTCPAndUDP", false)

	// Length of the response for 18 A records will be 542 bytes and UDPSize is 515.
	// Overflow error will be occur and truncated response will be returned with TC bit set for UDP protocol.
	testCoreDNSOverflow(t, "example.org.", 18, 515, "PreferUDP", true)
	testCoreDNSOverflow(t, "example.org.", 18, 515, "ForceTCP", false)
	testCoreDNSOverflow(t, "example.org.", 18, 515, "NoOptionsSpecified", true)
	testCoreDNSOverflow(t, "example.org.", 18, 515, "BothTCPAndUDP", false)

	// Testing with 15 characters in the domain name with UDPSize equal to 512.
	// -----------------------------------------------------------------------------------------------------------
	// Length of the response for 14 A records will be 482 bytes and UDPSize is 512.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example123.org.", 14, 512, "PreferUDP", false)
	testCoreDNSOverflow(t, "example123.org.", 14, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "example123.org.", 14, 512, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example123.org.", 14, 512, "BothTCPAndUDP", false)

	// Length of the response for 15 A records will be 512 bytes and UDPSize is 512.
	// This is a case where the response len is 515 bytes and protocol will be switched to TCP.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example123.org.", 15, 512, "PreferUDP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 512, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 512, "BothTCPAndUDP", false)

	// Length of the response for 16 A records will be 542 bytes and UDPSize is 512.
	// Overflow error will be occur and truncated response will be returned with TC bit set for UDP protocol.
	testCoreDNSOverflow(t, "example123.org.", 16, 512, "PreferUDP", true)
	testCoreDNSOverflow(t, "example123.org.", 16, 512, "ForceTCP", false)
	testCoreDNSOverflow(t, "example123.org.", 16, 512, "NoOptionsSpecified", true)
	testCoreDNSOverflow(t, "example123.org.", 16, 512, "BothTCPAndUDP", false)

	// Case where UDPSize is less than 512.
	// -----------------------------------------------------------------------------------------------------------
	// Minimum UDPSize is 512.
	// Length of the response for 15 A records will be 512 bytes and UDPSize is 512.
	// This is a case where the response len is 515 bytes and protocol will be switched to TCP.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example123.org.", 15, 511, "PreferUDP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 511, "ForceTCP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 511, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 511, "BothTCPAndUDP", false)

	// Case where UDPSize is greater than 512.
	// -----------------------------------------------------------------------------------------------------------
	// Length of the response for 15 A records will be 512 bytes and UDPSize is 513.
	// Minimum UDPSize is 512, but we are overriding it to 513.
	// Response is not truncated and TC bit is not set.
	testCoreDNSOverflow(t, "example123.org.", 15, 513, "PreferUDP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 513, "ForceTCP", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 513, "NoOptionsSpecified", false)
	testCoreDNSOverflow(t, "example123.org.", 15, 513, "BothTCPAndUDP", false)

	// Few cases where response is truncated.
	// -----------------------------------------------------------------------------------------------------------
	testCoreDNSOverflow(t, "e.", 29, 512, "PreferUDP", true)
	testCoreDNSOverflow(t, "ex.", 28, 524, "PreferUDP", true)
	testCoreDNSOverflow(t, "exa.", 27, 534, "PreferUDP", true)
	testCoreDNSOverflow(t, "exam.", 26, 542, "PreferUDP", true)
	testCoreDNSOverflow(t, "examp.", 25, 548, "PreferUDP", true)
	testCoreDNSOverflow(t, "exampl.", 24, 552, "PreferUDP", true)
	testCoreDNSOverflow(t, "example.", 23, 554, "PreferUDP", true)
	testCoreDNSOverflow(t, "example.org.", 19, 542, "PreferUDP", true)
}

func testCoreDNSOverflow(t *testing.T, domainName string, noOfARecords int, eDNSUDPSize uint16, proto string, expectTruncated bool) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		answers := []dns.RR{}

		// Add the desired number of A records to the response.
		for i := 0; i < noOfARecords; i++ {
			answers = append(answers, test.A(""+domainName+" IN A 127.0.0."+strconv.Itoa(1+i)))
		}
		ret.Answer = answers
		w.WriteMsg(ret)
	})
	defer s.Close()

	// When testing for AKS scenario, use upstream Azure DNS server address "168.63.129.16:53" instead of s.Addr
	p := NewProxy("TestCoreDNSOverflow", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	p.Start(5 * time.Second)
	defer p.Stop()
	t.Helper()

	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(domainName, dns.TypeA)
	recorder := dnstest.NewRecorder(&test.ResponseWriter{})
	request := request.Request{Req: queryMsg, W: recorder}
	request.Req.SetEdns0(eDNSUDPSize, true)

	protocolOpt := Options{}
	switch proto {
	case "PreferUDP":
		protocolOpt.PreferUDP = true
	case "ForceTCP":
		protocolOpt.ForceTCP = true
	case "BothTCPAndUDP":
		protocolOpt.PreferUDP = true
		protocolOpt.ForceTCP = true
	}

	// Call the function defined in connect.go
	response, err := p.Connect(context.Background(), request, protocolOpt)
	if err != nil {
		t.Errorf("Failed to connect to testdnsserver: %s", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Errorf("Test - DomainName : %s for No of A records : %v with eDNSUDPSize : %v , Expected No error, but got %v", domainName, noOfARecords, eDNSUDPSize, response.Rcode)
	}
	if response.Truncated != expectTruncated {
		t.Errorf("Test - DomainName : %s for No of A records : %v with eDNSUDPSize : %v , Expected truncated response for %s, but got TC flag %v", domainName, noOfARecords, eDNSUDPSize, proto, response.Truncated)
	}
}
