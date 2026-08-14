package siit

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
			},
		},
		{
			// name exists, but AAAA record is not synthesized
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
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 2001:db8::1"),
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
				Ns:       []dns.RR{test.SOA("example.com. 3600 IN SOA foo bar 1 7200 900 1209600 86400")},
			},
		},
		{
			// name exists, but AAAA records are a mix of synthesized and not
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
				Answer: []dns.RR{
					test.AAAA("example.com. 60 IN AAAA 2001:db8::1"),
					test.AAAA("example.com. 60 IN AAAA 64:ff9b::192.0.2.42"),
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
				},
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

func TestDoSIITNegativeResponse(t *testing.T) {
	origResponse := new(dns.Msg)
	origResponse.SetQuestion("example.org.", dns.TypeA)
	origResponse.Rcode = dns.RcodeSuccess // the client's original A answer

	aaaaFailure := new(dns.Msg)
	aaaaFailure.SetQuestion("example.org.", dns.TypeAAAA) // internal AAAA question
	aaaaFailure.Rcode = dns.RcodeServerFailure            // upstream AAAA lookup SERVFAILs
	aaaaFailure.RecursionAvailable = true

	d := &SIIT{
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaFailure,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	r.Id = 42
	w := &test.ResponseWriter{}

	out, synthesized, err := d.DoSIIT(context.Background(), w, r, origResponse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthesized {
		t.Errorf("expected synthesized=false, no A record was actually produced")
	}
	if out == aaaaFailure {
		t.Fatalf("expected DoSIIT to build a new response addressed to the client, not return the internal AAAA lookup response as-is")
	}
	if out.Rcode != aaaaFailure.Rcode {
		t.Errorf("expected Rcode %v to be carried over from the lookup, got %v", aaaaFailure.Rcode, out.Rcode)
	}
	if out.Id != r.Id {
		t.Errorf("expected response ID %v to match the client's request, got %v", r.Id, out.Id)
	}
	if len(out.Question) != 1 || out.Question[0].Qtype != dns.TypeA || out.Question[0].Name != "example.org." {
		t.Fatalf("expected the original A question to be preserved in the response, got %+v", out.Question)
	}
	if !out.RecursionAvailable {
		t.Errorf("expected RecursionAvailable to be carried over from the lookup response")
	}
}

func TestDoSIITNXDOMAIN(t *testing.T) {
	origResponse := new(dns.Msg)
	origResponse.SetQuestion("example.org.", dns.TypeA)
	origResponse.Rcode = dns.RcodeSuccess

	soa := test.SOA("example.org. 3600 IN SOA foo bar 1 7200 900 1209600 86400")

	aaaaNX := new(dns.Msg)
	aaaaNX.SetQuestion("example.org.", dns.TypeAAAA)
	aaaaNX.Rcode = dns.RcodeNameError
	aaaaNX.Authoritative = true
	aaaaNX.Ns = []dns.RR{soa}

	d := &SIIT{
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaNX,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	r.Id = 43
	w := &test.ResponseWriter{}

	out, synthesized, err := d.DoSIIT(context.Background(), w, r, origResponse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthesized {
		t.Errorf("expected synthesized=false, no A record was actually produced")
	}
	if out == aaaaNX {
		t.Fatalf("expected DoSIIT to build a new response addressed to the client, not return the internal AAAA lookup response as-is")
	}
	if out.Rcode != aaaaNX.Rcode {
		t.Errorf("expected Rcode %v from lookup, got %v", aaaaNX.Rcode, out.Rcode)
	}
	if out.Id != r.Id {
		t.Errorf("expected response ID %v to match the client's request, got %v", r.Id, out.Id)
	}
	if len(out.Question) != 1 || out.Question[0].Qtype != dns.TypeA || out.Question[0].Name != "example.org." {
		t.Fatalf("expected the original A question to be preserved in the response, got %+v", out.Question)
	}
	if !out.Authoritative {
		t.Errorf("expected Authoritative to be carried over from the lookup response")
	}
	if !reflect.DeepEqual(out.Ns, aaaaNX.Ns) {
		t.Errorf("expected authority section (SOA) to be carried over, got %+v", out.Ns)
	}
}

// TestDoSIITNilResponse covers Upstream.Lookup returning (nil, nil): DoSIIT
// must surface this as an error rather than pass a nil message on to a
// caller that will dereference it.
func TestDoSIITNilResponse(t *testing.T) {
	origResponse := new(dns.Msg)
	origResponse.SetQuestion("example.org.", dns.TypeA)
	origResponse.Rcode = dns.RcodeSuccess

	d := &SIIT{
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  nil, // simulate a well-behaved Upstream returning (nil, nil)
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	w := &test.ResponseWriter{}

	out, synthesized, err := d.DoSIIT(context.Background(), w, r, origResponse)
	if err == nil {
		t.Fatal("expected an error for a nil upstream response, got nil")
	}
	if synthesized {
		t.Errorf("expected synthesized=false, no A record was actually produced")
	}
	if out != nil {
		t.Errorf("expected a nil message alongside the error, got %+v", out)
	}
}

func TestServeDNSUpstreamSERVFAIL(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	initResp := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               42,
			Opcode:           dns.OpcodeQuery,
			RecursionDesired: true,
			Rcode:            dns.RcodeSuccess,
			Response:         true,
		},
		Question: []dns.Question{{Name: "example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
	}

	aaaaFailure := new(dns.Msg)
	aaaaFailure.SetQuestion("example.org.", dns.TypeAAAA)
	aaaaFailure.Rcode = dns.RcodeServerFailure
	aaaaFailure.RecursionAvailable = true

	d := &SIIT{
		Next:       &fakeHandler{t, initResp},
		IPv6Prefix: prefix,
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaFailure,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	r.Id = 42
	r.RecursionDesired = true

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	rc, err := d.ServeDNS(context.Background(), rec, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL rcode, got %v", rc)
	}
	if rec.Msg == nil {
		t.Fatal("expected a response to be written")
	}
	if rec.Msg.Id != r.Id {
		t.Errorf("expected response ID %v to match the client's request, got %v", r.Id, rec.Msg.Id)
	}
	if len(rec.Msg.Question) != 1 || rec.Msg.Question[0].Qtype != dns.TypeA || rec.Msg.Question[0].Name != "example.org." {
		t.Fatalf("expected the original A question to be preserved in the response, got %+v", rec.Msg.Question)
	}
	if !rec.Msg.RecursionAvailable {
		t.Errorf("expected RecursionAvailable to be carried over from the internal AAAA lookup")
	}
}

func TestServeDNSUpstreamNXDOMAIN(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	initResp := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               7,
			Opcode:           dns.OpcodeQuery,
			RecursionDesired: true,
			Rcode:            dns.RcodeSuccess,
			Response:         true,
		},
		Question: []dns.Question{{Name: "example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
	}

	soa := test.SOA("example.org. 3600 IN SOA foo bar 1 7200 900 1209600 86400")
	aaaaNX := new(dns.Msg)
	aaaaNX.SetQuestion("example.org.", dns.TypeAAAA)
	aaaaNX.Rcode = dns.RcodeNameError
	aaaaNX.Authoritative = true
	aaaaNX.Ns = []dns.RR{soa}

	d := &SIIT{
		Next:       &fakeHandler{t, initResp},
		IPv6Prefix: prefix,
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  aaaaNX,
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	r.Id = 7
	r.RecursionDesired = true

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	rc, err := d.ServeDNS(context.Background(), rec, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN rcode, got %v", rc)
	}
	if len(rec.Msg.Question) != 1 || rec.Msg.Question[0].Qtype != dns.TypeA {
		t.Fatalf("expected the original A question to be preserved, got %+v", rec.Msg.Question)
	}
	if !rec.Msg.Authoritative {
		t.Errorf("expected Authoritative to be carried over from the internal AAAA lookup")
	}
	if !reflect.DeepEqual(rec.Msg.Ns, aaaaNX.Ns) {
		t.Errorf("expected authority (SOA) to be carried over, got %+v", rec.Msg.Ns)
	}
}

func TestServeDNSUpstreamNilResponse(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	initResp := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               99,
			Opcode:           dns.OpcodeQuery,
			RecursionDesired: true,
			Rcode:            dns.RcodeSuccess,
			Response:         true,
		},
		Question: []dns.Question{{Name: "example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
	}

	d := &SIIT{
		Next:       &fakeHandler{t, initResp},
		IPv6Prefix: prefix,
		Upstream: &fakeUpstream{
			t:     t,
			qname: "example.org.",
			resp:  nil, // simulate a well-behaved Upstream returning (nil, nil)
		},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeA)
	r.Id = 99
	r.RecursionDesired = true

	rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
	rc, err := d.ServeDNS(context.Background(), rec, r)
	if err == nil {
		t.Fatal("expected an error from ServeDNS for a nil upstream response")
	}
	if rc != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL rcode, got %v", rc)
	}
}

// TestSynthesizePreservesAuthoritativeAndRecursive verifies RFC 6147 §5.4:
// the synthesized response's AA/RA flags must reflect the secondary AAAA
// response (where the data actually came from), not be unconditionally
// cleared by SetReply.
func TestSynthesizePreservesAuthoritativeAndRecursive(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	cases := []struct {
		name          string
		authoritative bool
		recursive     bool
	}{
		{"authoritative only", true, false},
		{"recursive only", false, true},
		{"both", true, true},
		{"neither", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &SIIT{IPv6Prefix: prefix}

			origReq := new(dns.Msg)
			origReq.SetQuestion("example.org.", dns.TypeA)
			origReq.Id = 55

			origResponse := new(dns.Msg)
			origResponse.SetReply(origReq)

			resp := new(dns.Msg)
			resp.SetQuestion("example.org.", dns.TypeAAAA)
			resp.Rcode = dns.RcodeSuccess
			resp.Authoritative = tc.authoritative
			resp.RecursionAvailable = tc.recursive
			resp.AuthenticatedData = true // must NOT survive synthesis
			resp.Answer = []dns.RR{
				test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.1"),
			}

			out, _ := d.Synthesize(origReq, origResponse, resp)

			if out.Authoritative != tc.authoritative {
				t.Errorf("Authoritative: expected %v, got %v", tc.authoritative, out.Authoritative)
			}
			if out.RecursionAvailable != tc.recursive {
				t.Errorf("RecursionAvailable: expected %v, got %v", tc.recursive, out.RecursionAvailable)
			}
			if out.AuthenticatedData {
				t.Errorf("expected AuthenticatedData to be cleared on a synthesized response, got true")
			}
			if out.Id != origReq.Id {
				t.Errorf("expected ID %v to be preserved from the original request, got %v", origReq.Id, out.Id)
			}
			if len(out.Question) != 1 || out.Question[0].Qtype != dns.TypeA {
				t.Fatalf("expected original A question to be preserved, got %+v", out.Question)
			}
		})
	}
}

// TestServeDNSMetricCounting verifies RequestsTranslatedCount is
// incremented exactly when a synthetic A record was actually produced, and
// left untouched for the secondary-failure and unmapped-only cases that
// previously overcounted.
func TestServeDNSMetricCounting(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("64:ff9b::/96")

	emptyAResp := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id: 1, Opcode: dns.OpcodeQuery, RecursionDesired: true,
			Rcode: dns.RcodeSuccess, Response: true,
		},
		Question: []dns.Question{{Name: "example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
	}

	cases := []struct {
		name         string
		aaaaResp     *dns.Msg
		wantIncrease float64
	}{
		{
			name: "successful synthesis increments",
			aaaaResp: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.org.", dns.TypeAAAA)
				m.Rcode = dns.RcodeSuccess
				m.Answer = []dns.RR{test.AAAA("example.org. 60 IN AAAA 64:ff9b::192.0.2.1")}
				return m
			}(),
			wantIncrease: 1,
		},
		{
			name: "secondary SERVFAIL does not increment",
			aaaaResp: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.org.", dns.TypeAAAA)
				m.Rcode = dns.RcodeServerFailure
				return m
			}(),
			wantIncrease: 0,
		},
		{
			name: "unmapped-only AAAA does not increment",
			aaaaResp: func() *dns.Msg {
				m := new(dns.Msg)
				m.SetQuestion("example.org.", dns.TypeAAAA)
				m.Rcode = dns.RcodeSuccess
				m.Answer = []dns.RR{test.AAAA("example.org. 60 IN AAAA 2001:db8::1")} // outside prefix, no eam
				return m
			}(),
			wantIncrease: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &SIIT{
				Next:       &fakeHandler{t, emptyAResp},
				IPv6Prefix: prefix,
				Upstream:   &fakeUpstream{t, "example.org.", tc.aaaaResp},
			}

			r := new(dns.Msg)
			r.SetQuestion("example.org.", dns.TypeA)
			ctx := context.Background()

			label := metrics.WithServer(ctx)
			before := testutil.ToFloat64(RequestsTranslatedCount.WithLabelValues(label))

			rec := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "::1"})
			if _, err := d.ServeDNS(ctx, rec, r); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			after := testutil.ToFloat64(RequestsTranslatedCount.WithLabelValues(label))
			if got := after - before; got != tc.wantIncrease {
				t.Errorf("expected counter to increase by %v, got %v (before=%v after=%v)", tc.wantIncrease, got, before, after)
			}
		})
	}
}

// embedIPv4 mirrors the RFC 6052 §2.2 embedding algorithm — the exact
// mirror image of extractIPv4 — so these tests can construct known-good
// NAT64 addresses independently of production code.
func embedIPv4(prefix net.IP, prefixLen int, v4 net.IP) net.IP {
	v4 = v4.To4()
	out := make([]byte, 16)
	copy(out, prefix.To16()[:prefixLen/8])

	i, j := prefixLen/8, 0
	for ; i < 8; i, j = i+1, j+1 {
		out[i] = v4[j]
	}
	if i == 8 {
		i++ // reserved "u" byte stays zero
	}
	for ; j < 4; i, j = i+1, j+1 {
		out[i] = v4[j]
	}
	return net.IP(out)
}

// TestExtractIPv4RFC6052Vectors uses the six literal example addresses from
// RFC 6052 §2.2 Table 1 ("Text Representation of IPv4-Embedded IPv6
// Addresses Using Network-Specific Prefixes"), rather than round-tripping
// through embedIPv4. embedIPv4 mirrors the same byte-placement algorithm as
// extractIPv4, so a shared offset bug in both helpers could still round-trip
// successfully and this test would never catch it. These fixtures are
// copied verbatim from the RFC, independent of any code in this package.
func TestExtractIPv4RFC6052Vectors(t *testing.T) {
	want := net.ParseIP("192.0.2.33").To4()

	cases := []struct {
		prefixCIDR string // Network-Specific Prefix column
		addr       string // IPv4-embedded IPv6 address column
	}{
		{"2001:db8::/32", "2001:db8:c000:221::"},
		{"2001:db8:100::/40", "2001:db8:1c0:2:21::"},
		{"2001:db8:122::/48", "2001:db8:122:c000:2:2100::"},
		{"2001:db8:122:300::/56", "2001:db8:122:3c0:0:221::"},
		{"2001:db8:122:344::/64", "2001:db8:122:344:c0:2:2100::"},
		{"2001:db8:122:344::/96", "2001:db8:122:344::192.0.2.33"},
	}

	for _, tc := range cases {
		t.Run(tc.prefixCIDR, func(t *testing.T) {
			_, prefix, err := net.ParseCIDR(tc.prefixCIDR)
			if err != nil {
				t.Fatalf("bad prefix fixture %q: %v", tc.prefixCIDR, err)
			}
			addr := net.ParseIP(tc.addr)
			if addr == nil {
				t.Fatalf("bad address fixture %q", tc.addr)
			}

			got := extractIPv4(addr, prefix)
			if !got.Equal(want) {
				t.Errorf("prefix %s, address %s: expected %v, got %v", tc.prefixCIDR, tc.addr, want, got)
			}
		})
	}
}

// TestToRejectsNonzeroUOctet: an address
// whose reserved "u" byte (byte 8) is nonzero must not be algorithmically
// translated, even though it falls within the configured prefix.
func TestToRejectsNonzeroUOctet(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("2001:db8:122:344::/64")
	eam := map[string]net.IP{}

	// bits 64-71 (byte 8) = 0xff, nonzero -- must be rejected
	addr := net.ParseIP("2001:db8:122:344:ffc0:2:2100:0")

	a, mapped := to4(eam, prefix, addr)
	if mapped {
		t.Fatalf("expected nonzero-u address to be rejected, got mapped address %v", a)
	}
}

// TestToAcceptsZeroUOctet is the counterpart: a well-formed address with
// u == 0 for the same /64 prefix must still translate correctly.
func TestToAcceptsZeroUOctet(t *testing.T) {
	_, prefix, _ := net.ParseCIDR("2001:db8:122:344::/64")
	eam := map[string]net.IP{}
	v4 := net.ParseIP("192.0.2.33").To4()

	addr := embedIPv4(net.ParseIP("2001:db8:122:344::"), 64, v4)

	a, mapped := to4(eam, prefix, addr)
	if !mapped {
		t.Fatalf("expected zero-u address to translate")
	}
	if !a.Equal(v4) {
		t.Errorf("expected %v, got %v", v4, a)
	}
}
