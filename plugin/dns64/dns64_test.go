package dns64

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func To6(prefix, address string) (net.IP, error) {
	_, pref, _ := net.ParseCIDR(prefix)
	addr := net.ParseIP(address)

	return to6(pref, addr)
}

func TestRequestShouldIntercept(t *testing.T) {
	tests := []struct {
		name      string
		allowIpv4 bool
		remoteIP  string
		msg       *dns.Msg
		want      bool
	}{
		{
			name:      "should intercept request from IPv6 network - AAAA - IN",
			allowIpv4: true,
			remoteIP:  "::1",
			msg:       new(dns.Msg).SetQuestion("example.com", dns.TypeAAAA),
			want:      true,
		},
		{
			name:      "should intercept request from IPv4 network - AAAA - IN",
			allowIpv4: true,
			remoteIP:  "127.0.0.1",
			msg:       new(dns.Msg).SetQuestion("example.com", dns.TypeAAAA),
			want:      true,
		},
		{
			name:      "should not intercept request from IPv4 network - AAAA - IN",
			allowIpv4: false,
			remoteIP:  "127.0.0.1",
			msg:       new(dns.Msg).SetQuestion("example.com", dns.TypeAAAA),
			want:      false,
		},
		{
			name:      "should not intercept request from IPv6 network - A - IN",
			allowIpv4: false,
			remoteIP:  "::1",
			msg:       new(dns.Msg).SetQuestion("example.com", dns.TypeA),
			want:      false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := DNS64{AllowIPv4: tc.allowIpv4}
			rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: tc.remoteIP})
			r := request.Request{W: rec, Req: tc.msg}

			actual := h.requestShouldIntercept(&r)

			if actual != tc.want {
				t.Fatalf("Expected %v, but got %v", tc.want, actual)
			}
		})
	}
}

func TestTo6(t *testing.T) {
	v6, err := To6("64:ff9b::/96", "64.64.64.64")
	if err != nil {
		t.Error(err)
	}
	if v6.String() != "64:ff9b::4040:4040" {
		t.Errorf("%d", v6)
	}

	v6, err = To6("64:ff9b::/64", "64.64.64.64")
	if err != nil {
		t.Error(err)
	}
	if v6.String() != "64:ff9b::40:4040:4000:0" {
		t.Errorf("%d", v6)
	}

	v6, err = To6("64:ff9b::/56", "64.64.64.64")
	if err != nil {
		t.Error(err)
	}
	if v6.String() != "64:ff9b:0:40:40:4040::" {
		t.Errorf("%d", v6)
	}

	v6, err = To6("64::/32", "64.64.64.64")
	if err != nil {
		t.Error(err)
	}
	if v6.String() != "64:0:4040:4040::" {
		t.Errorf("%d", v6)
	}
}

func TestResponseShould(t *testing.T) {
	var tests = []struct {
		resp         dns.Msg
		translateAll bool
		expected     bool
	}{
		// If there's an AAAA record, then no
		{
			resp: dns.Msg{
				MsgHdr: dns.MsgHdr{
					Rcode: dns.RcodeSuccess,
				},
				Answer: []dns.RR{
					test.AAAA("example.com. IN AAAA ::1"),
				},
			},
			expected: false,
		},
		// If there's no AAAA, then true
		{
			resp: dns.Msg{
				MsgHdr: dns.MsgHdr{
					Rcode: dns.RcodeSuccess,
				},
				Ns: []dns.RR{
					test.SOA("example.com. IN SOA foo bar 1 1 1 1 1"),
				},
			},
			expected: true,
		},
		// Failure, except NameError, should be true
		{
			resp: dns.Msg{
				MsgHdr: dns.MsgHdr{
					Rcode: dns.RcodeNotImplemented,
				},
				Ns: []dns.RR{
					test.SOA("example.com. IN SOA foo bar 1 1 1 1 1"),
				},
			},
			expected: true,
		},
		// NameError should be false
		{
			resp: dns.Msg{
				MsgHdr: dns.MsgHdr{
					Rcode: dns.RcodeNameError,
				},
				Ns: []dns.RR{
					test.SOA("example.com. IN SOA foo bar 1 1 1 1 1"),
				},
			},
			expected: false,
		},
		// If there's an AAAA record, but translate_all is configured, then yes
		{
			resp: dns.Msg{
				MsgHdr: dns.MsgHdr{
					Rcode: dns.RcodeSuccess,
				},
				Answer: []dns.RR{
					test.AAAA("example.com. IN AAAA ::1"),
				},
			},
			translateAll: true,
			expected:     true,
		},
	}

	d := DNS64{}

	for idx, tc := range tests {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			d.TranslateAll = tc.translateAll
			actual := d.responseShouldDNS64(&tc.resp)
			if actual != tc.expected {
				t.Fatalf("Expected %v got %v", tc.expected, actual)
			}
		})
	}
}

func TestDNS64(t *testing.T) {
	var cases = []struct {
		// a brief summary of the test case
		name string

		// the request
		req *dns.Msg

		// the initial response from the "downstream" server
		initResp *dns.Msg

		// A response to provide
		aResp *dns.Msg

		// the expected ultimate result
		resp *dns.Msg
	}{
		{
			// no AAAA record, yes A record. Do DNS64
			name: "standard flow",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 70 IN SOA foo bar 1 1 1 1 1")},
			},
			aResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               43,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					test.A("example.com. 5000 IN A 192.0.2.43"),
				},
			},

			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					// override RR ttl to SOA ttl, since it's lower
					test.AAAA("example.com. 70 IN AAAA 64:ff9b::192.0.2.43"),
				},
			},
		},
		{
			// name exists, but has neither A nor AAAA record
			name: "a empty",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},
			aResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               43,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},

			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
				Answer:   []dns.RR{}, // just to make comparison happy
			},
		},
		{
			// Query error other than NameError
			name: "non-nxdomain error",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ // failure
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeRefused,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			aResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               43,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					test.A("example.com. 5000 IN A 192.0.2.43"),
				},
			},

			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					test.AAAA("example.com. 600 IN AAAA 64:ff9b::192.0.2.43"),
				},
			},
		},
		{
			// nxdomain (NameError): don't even try an A request.
			name: "nxdomain",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ // failure
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeNameError,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},
			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeNameError,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},
		},
		{
			// AAAA record exists
			name: "AAAA record",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},

			initResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA ::1"),
					test.AAAA("example.com. 5000 IN AAAA ::2"),
				},
			},

			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA ::1"),
					test.AAAA("example.com. 5000 IN AAAA ::2"),
				},
			},
		},
		{
			// no AAAA records, A record response truncated.
			name: "truncated A response",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 70 IN SOA foo bar 1 1 1 1 1")},
			},
			aResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               43,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Truncated:        true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					test.A("example.com. 5000 IN A 192.0.2.43"),
				},
			},

			resp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Truncated:        true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					// override RR ttl to SOA ttl, since it's lower
					test.AAAA("example.com. 70 IN AAAA 64:ff9b::192.0.2.43"),
				},
			},
		},
	}

	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")

	for idx, tc := range cases {
		t.Run(fmt.Sprintf("%d_%s", idx, tc.name), func(t *testing.T) {
			d := DNS64{
				Next:     &fakeHandler{t, tc.initResp},
				Prefix:   pfx,
				Upstream: &fakeUpstream{t: t, qname: tc.req.Question[0].Name, resp: tc.aResp},
			}

			rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
			rc, err := d.ServeDNS(context.Background(), rec, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			actual := rec.Msg
			if actual.Rcode != rc {
				t.Fatalf("ServeDNS should return real result code %q != %q", actual.Rcode, rc)
			}

			if !reflect.DeepEqual(actual, tc.resp) {
				t.Fatalf("Final answer should match expected %q != %q", actual, tc.resp)
			}
		})
	}
}

type fakeHandler struct {
	t     *testing.T
	reply *dns.Msg
}

func (fh *fakeHandler) ServeDNS(_ context.Context, w dns.ResponseWriter, _ *dns.Msg) (int, error) {
	if fh.reply == nil {
		panic("fakeHandler ServeDNS with nil reply")
	}
	w.WriteMsg(fh.reply)

	return fh.reply.Rcode, nil
}
func (fh *fakeHandler) Name() string {
	return "fake"
}

type fakeUpstream struct {
	t     *testing.T
	qname string
	resp  *dns.Msg // A-query response
	soa   *dns.Msg // optional SOA-query response; nil means "no SOA found"
}

// TestSynthesizeEDE asserts that synthesised AAAA responses carry EDE code 29
// ("Synthesized") when the client's query included OPT, and that we do not add
// OPT (or EDE) when the client did not opt into EDNS.
func TestSynthesizeEDE(t *testing.T) {
	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")

	aResp := &dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: 43, Response: true, Rcode: dns.RcodeSuccess},
		Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
		Answer:   []dns.RR{test.A("v4only.example. 60 IN A 192.0.2.42")},
	}
	emptyAAAA := &dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: 42, Response: true, Rcode: dns.RcodeSuccess},
		Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
		Ns:       []dns.RR{test.SOA("v4only.example. 70 IN SOA foo bar 1 1 1 1 1")},
	}
	newDNS64 := func() *DNS64 {
		return &DNS64{
			Next:     &fakeHandler{t, emptyAAAA},
			Prefix:   pfx,
			Upstream: &fakeUpstream{t: t, qname: "v4only.example.", resp: aResp},
		}
	}

	t.Run("EDNS-aware AAAA query gets EDE 29 Synthesized", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}}}
		req.SetEdns0(4096, false)
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		opt := rec.Msg.IsEdns0()
		if opt == nil {
			t.Fatal("expected OPT with EDE, got no OPT")
		}
		found := false
		for _, o := range opt.Option {
			if ede, ok := o.(*dns.EDNS0_EDE); ok && ede.InfoCode == dns.ExtendedErrorCodeSynthesized {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected EDE InfoCode %d (Synthesized), got %#v", dns.ExtendedErrorCodeSynthesized, opt.Option)
		}
	})

	t.Run("non-EDNS AAAA query stays non-EDNS", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rec.Msg.IsEdns0() != nil {
			t.Fatal("response must not include OPT when the query had no OPT")
		}
	})
}

// TestFilterA covers the filter_a option. Subtests assert:
//   - external A queries → NOERROR/NODATA + EDE 17 (when EDNS present).
//   - AAAA queries still synthesise (internal A lookup bypasses the filter
//     via dns64InternalKey).
//   - internal A lookups carrying dns64InternalKey are passed through.
//   - RFC 6891: no OPT in query → no OPT in response.
//   - RFC 2308: the internal SOA lookup populates the authority section, from
//     either Answer (zone apex) or Ns (non-apex), with a graceful fallback
//     when the lookup returns no SOA.
//   - non-INET class is not filtered.
func TestFilterA(t *testing.T) {
	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")

	aResp := &dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: 43, Response: true, Rcode: dns.RcodeSuccess},
		Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
		Answer:   []dns.RR{test.A("v4only.example. 60 IN A 192.0.2.42")},
	}
	emptyAAAA := &dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: 42, Response: true, Rcode: dns.RcodeSuccess},
		Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
		Ns:       []dns.RR{test.SOA("v4only.example. 70 IN SOA foo bar 1 1 1 1 1")},
	}

	newDNS64 := func() *DNS64 {
		return &DNS64{
			Next:         &fakeHandler{t, emptyAAAA},
			Prefix:       pfx,
			AllowIPv4:    true,
			FilterA:      true,
			TranslateAll: true,
			Upstream:     &fakeUpstream{t: t, qname: "v4only.example.", resp: aResp},
		}
	}

	t.Run("external A suppressed with EDE 17", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		req.SetEdns0(4096, false) // EDE is only emitted when the client signalled EDNS.
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "127.0.0.1"})

		rc, err := d.ServeDNS(context.Background(), rec, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rc != dns.RcodeSuccess {
			t.Fatalf("rcode: want NOERROR, got %s", dns.RcodeToString[rc])
		}
		if len(rec.Msg.Answer) != 0 {
			t.Fatalf("answer section: want empty, got %v", rec.Msg.Answer)
		}
		opt := rec.Msg.IsEdns0()
		if opt == nil {
			t.Fatal("expected OPT record carrying EDE, got none")
		}
		found := false
		for _, o := range opt.Option {
			if ede, ok := o.(*dns.EDNS0_EDE); ok && ede.InfoCode == dns.ExtendedErrorCodeFiltered {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected EDE InfoCode %d (Filtered) in options, got %#v", dns.ExtendedErrorCodeFiltered, opt.Option)
		}
	})

	t.Run("AAAA still synthesised when filter_a is set", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})

		rc, err := d.ServeDNS(context.Background(), rec, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rc != dns.RcodeSuccess {
			t.Fatalf("rcode: want NOERROR, got %s", dns.RcodeToString[rc])
		}
		if len(rec.Msg.Answer) != 1 {
			t.Fatalf("want 1 synthesised AAAA, got %v", rec.Msg.Answer)
		}
		aaaa, ok := rec.Msg.Answer[0].(*dns.AAAA)
		if !ok {
			t.Fatalf("want AAAA RR, got %T", rec.Msg.Answer[0])
		}
		want := net.ParseIP("64:ff9b::192.0.2.42")
		if !aaaa.AAAA.Equal(want) {
			t.Fatalf("want %s, got %s", want, aaaa.AAAA)
		}
	})

	t.Run("internal A lookup bypasses filter_a", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})

		// Marked exactly as DoDNS64 does before re-entering the chain.
		ctx := context.WithValue(context.Background(), dns64InternalKey{}, true)

		// fakeHandler's reply is empty-AAAA — not useful here. Swap for the A
		// response so we can assert the internal call passes through.
		d.Next = &fakeHandler{t, aResp}

		if _, err := d.ServeDNS(ctx, rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(rec.Msg.Answer) != 1 || rec.Msg.Answer[0].Header().Rrtype != dns.TypeA {
			t.Fatalf("internal A lookup should have passed through to next with an A record, got %v", rec.Msg.Answer)
		}
	})

	t.Run("no OPT in query means no OPT in response", func(t *testing.T) {
		// RFC 6891 forbids OPT in a response when the query had no OPT.
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "127.0.0.1"})
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rec.Msg.IsEdns0() != nil {
			t.Fatal("response must not include OPT when the query had no OPT")
		}
	})

	t.Run("SOA from Authority section is included for RFC 2308 negative caching", func(t *testing.T) {
		d := newDNS64()
		// Upstream returns NODATA for SOA-at-non-apex with the zone's SOA in Ns.
		d.Upstream = &fakeUpstream{
			t:     t,
			qname: "v4only.example.",
			soa: &dns.Msg{
				MsgHdr: dns.MsgHdr{Response: true, Rcode: dns.RcodeSuccess},
				Ns:     []dns.RR{test.SOA("example. 3600 IN SOA ns.example. hostmaster.example. 1 7200 900 1209600 86400")},
			},
		}
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		req.SetEdns0(4096, false)
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "127.0.0.1"})
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(rec.Msg.Ns) != 1 {
			t.Fatalf("want 1 SOA in authority section, got %d: %v", len(rec.Msg.Ns), rec.Msg.Ns)
		}
		soa, ok := rec.Msg.Ns[0].(*dns.SOA)
		if !ok {
			t.Fatalf("want SOA RR in authority section, got %T", rec.Msg.Ns[0])
		}
		if soa.Hdr.Name != "example." {
			t.Fatalf("want SOA name example., got %s", soa.Hdr.Name)
		}
	})

	t.Run("SOA from Answer section is included when the queried name is a zone apex", func(t *testing.T) {
		d := newDNS64()
		d.Upstream = &fakeUpstream{
			t:     t,
			qname: "apex.example.",
			soa: &dns.Msg{
				MsgHdr: dns.MsgHdr{Response: true, Rcode: dns.RcodeSuccess},
				Answer: []dns.RR{test.SOA("apex.example. 3600 IN SOA ns.apex.example. hostmaster.apex.example. 1 7200 900 1209600 86400")},
			},
		}
		req := &dns.Msg{Question: []dns.Question{{Name: "apex.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		req.SetEdns0(4096, false)
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "127.0.0.1"})
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(rec.Msg.Ns) != 1 {
			t.Fatalf("want 1 SOA in authority section, got %d", len(rec.Msg.Ns))
		}
	})

	t.Run("SOA lookup failure falls back to NODATA without SOA", func(t *testing.T) {
		d := newDNS64()
		// Upstream returns an empty message for SOA (no Answer, no Ns). Production
		// code treats this as "no SOA" and returns NODATA without Ns — it does
		// not SERVFAIL.
		d.Upstream = &fakeUpstream{t: t, qname: "v4only.example.", resp: aResp /* soa unset → empty SOA response */}
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "127.0.0.1"})
		rc, err := d.ServeDNS(context.Background(), rec, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rc != dns.RcodeSuccess {
			t.Fatalf("want NOERROR, got %s", dns.RcodeToString[rc])
		}
		if len(rec.Msg.Ns) != 0 {
			t.Fatalf("want no SOA when upstream didn't return one, got %v", rec.Msg.Ns)
		}
	})

	t.Run("non-INET A class is not filtered", func(t *testing.T) {
		d := newDNS64()
		req := &dns.Msg{Question: []dns.Question{{Name: "v4only.example.", Qtype: dns.TypeA, Qclass: dns.ClassCHAOS}}}
		rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
		// With filter_a skipped and not an AAAA, we just defer to Next.
		d.Next = &fakeHandler{t, aResp}
		if _, err := d.ServeDNS(context.Background(), rec, req); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// Presence of the A answer from fakeHandler proves filter_a didn't fire.
		if len(rec.Msg.Answer) != 1 {
			t.Fatalf("class CHAOS A should pass through unfiltered, got %v", rec.Msg.Answer)
		}
	})
}

func (fu *fakeUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	if fu.qname == "" {
		fu.t.Fatalf("Unexpected lookup for %s", name)
	}
	if name != fu.qname {
		fu.t.Fatalf("Wrong lookup for %s, expected %s", name, fu.qname)
	}

	switch typ {
	case dns.TypeA:
		return fu.resp, nil
	case dns.TypeSOA:
		// filter_a issues an SOA lookup to populate the authority section for
		// RFC 2308 negative caching. Tests that don't set soa get a synthetic
		// empty response — the production code treats that as "no SOA found"
		// and returns NODATA without a SOA, which is the correct fallback.
		if fu.soa != nil {
			return fu.soa, nil
		}
		return &dns.Msg{}, nil
	}
	fu.t.Fatalf("Wrong lookup type %d", typ)
	return nil, nil // unreachable (t.Fatalf halts the goroutine); required by the compiler
}
