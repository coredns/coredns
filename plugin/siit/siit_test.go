package siit

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

// TestToUnmappedAAAA reproduces the empty-RDATA-A-record bug: an AAAA
// address that matches neither an EAM entry nor ipv6_prefix must not
// produce an A record at all.
func TestToUnmappedAAAA(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")
	eam := map[string]net.IP{}

	unrelated := net.ParseIP("2001:db8::1")
	a, mapped := to4(eam, prefix, unrelated)
	if mapped {
		t.Fatalf("expected no mapping for unrelated AAAA address, got %v", a)
	}
}

// TestToEamPrecedence reproduces the reversed-lookup-order bug: when an
// address matches both ipv6_prefix and an explicit eam entry, the eam
// entry must win.
func TestToEamPrecedence(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")
	eamAddr := net.ParseIP("64:ff9b::192.0.2.1")

	eam := make(map[string]net.IP)
	eam[eamAddr.String()] = net.ParseIP("203.0.113.9")

	a, mapped := to4(eam, prefix, eamAddr)
	if !mapped {
		t.Fatalf("expected eam mapping to be found")
	}
	want := net.ParseIP("203.0.113.9").To4()
	if !a.Equal(want) {
		t.Errorf("expected eam-mapped address %v, got %v (algorithmic translation was used instead)", want, a)
	}
}

// TestToAlgorithmicFallback verifies RFC 6052 translation still applies
// when no eam entry matches.
func TestToAlgorithmicFallback(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")
	eam := map[string]net.IP{}
	addr := net.ParseIP("64:ff9b::192.0.2.42")

	a, mapped := to4(eam, prefix, addr)
	if !mapped {
		t.Fatalf("expected algorithmic translation to apply")
	}
	want := net.ParseIP("192.0.2.42").To4()
	if !a.Equal(want) {
		t.Errorf("expected %v, got %v", want, a)
	}
}

func TestSIIT(t *testing.T) {
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
			// no A record, yes AAAA record. Do SIIT
			name: "standard flow",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					test.AAAA("example.com. 5000 IN AAAA 64:ff9b::192.0.2.43"),
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					// override RR ttl to SOA ttl, since it's lower
					test.A("example.com. 70 IN A 192.0.2.43"),
				},
			},
		},
		{
			// name exists, but has neither A nor AAAA record
			name: "aaaa empty",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ // failure
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeRefused,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			aResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               43,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					test.AAAA("example.com. 5000 IN AAAA 64:ff9b::192.0.2.43"),
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					test.A("example.com. 600 IN A 192.0.2.43"),
				},
			},
		},
		{
			// nxdomain (NameError): don't even try an AAAA request.
			name: "nxdomain",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ // failure
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeNameError,
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
					Rcode:            dns.RcodeNameError,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},
		},
		{
			// A record exists
			name: "A record",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},

			initResp: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 127.0.0.1"),
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 127.0.0.1"),
				},
			},
		},
		{
			// no A records, AAAA record response truncated.
			name: "truncated AAAA response",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
					test.AAAA("example.com. 5000 IN AAAA 64:ff9b::192.0.2.43"),
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 192.0.2.42"),
					// override RR ttl to SOA ttl, since it's lower
					test.A("example.com. 70 IN A 192.0.2.43"),
				},
			},
		},
		{
			// no A records, AAAA record response via eam.
			name: "eam AAAA response",
			req: &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:               42,
					RecursionDesired: true,
					Opcode:           dns.OpcodeQuery,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			initResp: &dns.Msg{ //success, no answers
				MsgHdr: dns.MsgHdr{
					Id:               42,
					Opcode:           dns.OpcodeQuery,
					RecursionDesired: true,
					Rcode:            dns.RcodeSuccess,
					Response:         true,
				},
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 64:dead::1"),
					test.AAAA("example.com. 5000 IN AAAA 64:dead::2"),
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
				Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer: []dns.RR{
					test.A("example.com. 60 IN A 10.0.0.1"),
					// override RR ttl to SOA ttl, since it's lower
					test.A("example.com. 70 IN A 10.0.0.2"),
				},
			},
		},
	}

	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")

	eam4 := make(map[string]net.IP)
	eam4["64:dead::1"] = net.ParseIP("10.0.0.1")
	eam4["64:dead::2"] = net.ParseIP("10.0.0.2")

	for idx, tc := range cases {
		t.Run(fmt.Sprintf("%d_%s", idx, tc.name), func(t *testing.T) {
			d := SIIT{
				Next:       &fakeHandler{t, tc.initResp},
				IPv6Prefix: pfx,
				Eam4:       eam4,
				Upstream:   &fakeUpstream{t, tc.req.Question[0].Name, tc.aResp},
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
	resp  *dns.Msg
}

func (fu *fakeUpstream) Lookup(_ context.Context, _ request.Request, name string, typ uint16) (*dns.Msg, error) {
	if fu.qname == "" {
		fu.t.Fatalf("Unexpected A lookup for %s", name)
	}
	if name != fu.qname {
		fu.t.Fatalf("Wrong A lookup for %s, expected %s", name, fu.qname)
	}

	if typ != dns.TypeA && typ != dns.TypeAAAA {
		fu.t.Fatalf("Wrong lookup type %d, expected %d or %d", typ, dns.TypeA, dns.TypeAAAA)
	}

	return fu.resp, nil
}

func TestDoSIITPassesThroughNegativeResponse(t *testing.T) {
	origResponse := new(dns.Msg)
	origResponse.SetQuestion("example.org.", dns.TypeA)
	origResponse.Rcode = dns.RcodeSuccess // the client's original A answer

	aaaaFailure := new(dns.Msg)
	aaaaFailure.Rcode = dns.RcodeServerFailure // upstream AAAA lookup SERVFAILs

	d := &SIIT{
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaFailure,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	w := &test.ResponseWriter{}

	out, err := d.DoSIIT(context.Background(), w, r, origResponse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != origResponse {
		t.Errorf("expected DoSIIT to pass through origResponse untouched, got a different message")
	}
	if out.Rcode != dns.RcodeSuccess {
		t.Errorf("expected original Rcode %v preserved, got %v", dns.RcodeSuccess, out.Rcode)
	}
}

func TestDoSIITPassesThroughNXDOMAIN(t *testing.T) {
	origResponse := new(dns.Msg)
	origResponse.SetQuestion("example.org.", dns.TypeA)
	origResponse.Rcode = dns.RcodeSuccess

	aaaaNX := new(dns.Msg)
	aaaaNX.Rcode = dns.RcodeNameError

	d := &SIIT{
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaNX,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	w := &test.ResponseWriter{}

	out, err := d.DoSIIT(context.Background(), w, r, origResponse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != origResponse {
		t.Errorf("expected pass-through on NXDOMAIN, got synthesized response")
	}
	if out.Rcode != dns.RcodeSuccess {
		t.Errorf("expected original Rcode %v preserved, got %v", dns.RcodeSuccess, out.Rcode)
	}
}
