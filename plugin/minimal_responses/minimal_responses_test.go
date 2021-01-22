package minimal_responses

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type responseStruct struct {
	answer []dns.RR
	ns     []dns.RR
	extra  []dns.RR
	rcode  int
}

func TestMinimizeResponse(t *testing.T) {
	baseAnswer := []dns.RR{
		test.A("google.com.  293 IN A 142.250.76.46"),
	}
	baseNs := []dns.RR{
		test.NS("google.com.  157127 IN NS ns2.google.com."),
		test.NS("google.com.  157127 IN NS ns1.google.com."),
		test.NS("google.com.  157127 IN NS ns3.google.com."),
		test.NS("google.com.  157127 IN NS ns4.google.com."),
	}

	baseExtra := []dns.RR{
		test.A("ns2.google.com. 316273 IN A 216.239.34.10"),
		test.AAAA("ns2.google.com. 157127 IN AAAA 2001:4860:4802:34::a"),
		test.A("ns3.google.com. 316274 IN A 216.239.36.10"),
		test.AAAA("ns3.google.com. 157127 IN AAAA 2001:4860:4802:36::a"),
		test.A("ns1.google.com. 165555 IN A 216.239.32.10"),
		test.AAAA("ns1.google.com. 165555 IN AAAA 2001:4860:4802:32::a"),
		test.A("ns4.google.com. 190188 IN A 216.239.38.10"),
		test.AAAA("ns4.google.com. 157127 IN AAAA 2001:4860:4802:38::a"),
	}

	tests := []struct {
		active   bool
		original responseStruct
		minimal  responseStruct
	}{
		{ // minimization possible NoError case
			original: responseStruct{
				answer: baseAnswer,
				ns:     nil,
				extra:  baseExtra,
				rcode: 0,
			},
			minimal: responseStruct{
				answer: baseAnswer,
				ns:     nil,
				extra:  nil,
				rcode: 0,
			},
		},
		{ // delegate response case
			original: responseStruct{
				answer: nil,
				ns:     baseNs,
				extra:  baseExtra,
				rcode: 0,
			},
			minimal: responseStruct{
				answer: nil,
				ns:     baseNs,
				extra:  baseExtra,
				rcode: 0,
			},
		},{ // negative response case
			original: responseStruct{
				answer: baseAnswer,
				ns:     baseNs,
				extra:  baseExtra,
				rcode: 2,
			},
			minimal: responseStruct{
				answer: baseAnswer,
				ns:     baseNs,
				extra:  baseExtra,
				rcode: 2,
			},
		},
	}

	for i, tc := range tests {
		req := new(dns.Msg)
		req.SetQuestion("google.com", dns.TypeA)
		req.Answer = tc.original.answer
		req.Extra = tc.original.extra
		req.Ns = tc.original.ns
		req.Rcode = tc.original.rcode

		o := &minimalResponse{}
		req = o.minimizeResponse(req)

		if len(tc.minimal.ns) != len(req.Ns) {
			t.Errorf("Test %d: Expected %d Ns, but got %d", i, len(tc.minimal.ns), len(req.Ns))
			continue
		}

		if len(tc.minimal.extra) != len(req.Extra) {
			t.Errorf("Test %d: Expected %d Extras, but got %d", i, len(tc.minimal.extra), len(req.Extra))
			continue
		}

		for j, a := range req.Ns {
			if tc.minimal.ns[j].String() != a.String() {
				t.Errorf("Test %d: Expected NS %d to be %v, but got %v", i, j, tc.minimal.ns[j], a)
			}
		}

		for j, a := range req.Extra {
			if tc.minimal.extra[j].String() != a.String() {
				t.Errorf("Test %d: Expected Extra %d to be %v, but got %v", i, j, tc.minimal.extra[j], a)
			}
		}
	}
}
