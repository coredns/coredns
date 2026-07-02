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

func To6(prefix, address string) (net.IP, error) {
	_, pref, _ := net.ParseCIDR(prefix)
	addr := net.ParseIP(address)
	eam := make(map[string]net.IP)

	return to6(pref, eam, addr)
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
			// no AAAA record, yes A record. Do SIIT
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
		{
			// no AAAA records, A record response via eam.
			name: "eam A response",
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
					test.A("example.com. 60 IN A 10.0.0.1"),
					test.A("example.com. 5000 IN A 10.0.0.2"),
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
					test.AAAA("example.com. 60 IN AAAA 64:dead::1"),
					// override RR ttl to SOA ttl, since it's lower
					test.AAAA("example.com. 70 IN AAAA 64:dead::2"),
				},
			},
		},
	}

	_, pfx, _ := net.ParseCIDR("64:ff9b::/96")

	eam6 := make(map[string]net.IP)
	eam6["10.0.0.1"] = net.ParseIP("64:dead::1")
	eam6["10.0.0.2"] = net.ParseIP("64:dead::2")

	eam4 := make(map[string]net.IP)
	eam4["64:dead::1"] = net.ParseIP("10.0.0.1")
	eam4["64:dead::2"] = net.ParseIP("10.0.0.2")

	for idx, tc := range cases {
		t.Run(fmt.Sprintf("%d_%s", idx, tc.name), func(t *testing.T) {
			d := SIIT{
				Next:       &fakeHandler{t, tc.initResp},
				IPv6Prefix: pfx,
				Eam6:       eam6,
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
