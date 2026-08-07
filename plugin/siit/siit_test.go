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
	if out != aaaaFailure {
		t.Errorf("expected DoSIIT to pass the lookup response, got a different message")
	}
	if out.Rcode != aaaaFailure.Rcode {
		t.Errorf("expected Rcode %v from lookup, got %v", aaaaFailure.Rcode, out.Rcode)
	}
}

func TestDoSIITNXDOMAIN(t *testing.T) {
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
	if out != aaaaNX {
		t.Errorf("expected DoSIIT to pass the lookup response, got a different message")
	}
	if out.Rcode != aaaaNX.Rcode {
		t.Errorf("expected Rcode %v from lookup, got %v", aaaaNX.Rcode, out.Rcode)
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

// TestExtractIPv4RFC6052Vectors covers all six valid RFC 6052 Table 1
// prefix lengths and verifies round-trip extraction.
func TestExtractIPv4RFC6052Vectors(t *testing.T) {
	v4 := net.ParseIP("192.0.2.33").To4()
	base := net.ParseIP("2001:db8::")

	for _, pl := range []int{32, 40, 48, 56, 64, 96} {
		t.Run(fmt.Sprintf("PL_%d", pl), func(t *testing.T) {
			addr := embedIPv4(base, pl, v4)
			_, prefix, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", base.String(), pl))

			got := extractIPv4(addr, prefix)
			if !got.Equal(v4) {
				t.Errorf("PL=%d: expected %v, got %v (embedded addr %v)", pl, v4, got, addr)
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
