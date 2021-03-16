package minimal

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// testHandler implements plugin.Handler and will be used to create a stub handler for the test
type testHandler struct {
	Response *test.Case
	Next     plugin.Handler
}

func (t *testHandler) Name() string { return "test-handler" }

func (t *testHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	d := new(dns.Msg)
	d.SetReply(r)
	if t.Response != nil {
		d.Answer = t.Response.Answer
		d.Ns = t.Response.Ns
		d.Extra = t.Response.Extra
		d.Rcode = t.Response.Rcode
	}
	w.WriteMsg(d)
	return 0, nil
}

func TestMinimizeResponse(t *testing.T) {
	baseAnswer := []dns.RR{
		test.A("example.com.  293 IN A 142.250.76.46"),
	}
	baseNs := []dns.RR{
		test.NS("example.com.  157127 IN NS ns2.example.com."),
		test.NS("example.com.  157127 IN NS ns1.example.com."),
		test.NS("example.com.  157127 IN NS ns3.example.com."),
		test.NS("example.com.  157127 IN NS ns4.example.com."),
	}

	baseExtra := []dns.RR{
		test.A("ns2.example.com. 316273 IN A 216.239.34.10"),
		test.AAAA("ns2.example.com. 157127 IN AAAA 2001:4860:4802:34::a"),
		test.A("ns3.example.com. 316274 IN A 216.239.36.10"),
		test.AAAA("ns3.example.com. 157127 IN AAAA 2001:4860:4802:36::a"),
		test.A("ns1.example.com. 165555 IN A 216.239.32.10"),
		test.AAAA("ns1.example.com. 165555 IN AAAA 2001:4860:4802:32::a"),
		test.A("ns4.example.com. 190188 IN A 216.239.38.10"),
		test.AAAA("ns4.example.com. 157127 IN AAAA 2001:4860:4802:38::a"),
	}

	tests := []struct {
		active   bool
		original test.Case
		minimal  test.Case
	}{
		{ // minimization possible NoError case
			original: test.Case{
				Answer: baseAnswer,
				Ns:     nil,
				Extra:  baseExtra,
				Rcode:  0,
			},
			minimal: test.Case{
				Answer: baseAnswer,
				Ns:     nil,
				Extra:  nil,
				Rcode:  0,
			},
		},
		{ // delegate response case
			original: test.Case{
				Answer: nil,
				Ns:     baseNs,
				Extra:  baseExtra,
				Rcode:  0,
			},
			minimal: test.Case{
				Answer: nil,
				Ns:     baseNs,
				Extra:  baseExtra,
				Rcode:  0,
			},
		}, { // negative response case
			original: test.Case{
				Answer: baseAnswer,
				Ns:     baseNs,
				Extra:  baseExtra,
				Rcode:  2,
			},
			minimal: test.Case{
				Answer: baseAnswer,
				Ns:     baseNs,
				Extra:  baseExtra,
				Rcode:  2,
			},
		},
	}

	for i, tc := range tests {
		req := new(dns.Msg)
		req.SetQuestion("example.com", dns.TypeA)

		tHandler := &testHandler{
			Response: &tc.original,
			Next:     nil,
		}
		o := &minimalHandler{Next: tHandler}
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := o.ServeDNS(context.TODO(), rec, req)

		if err != nil {
			t.Errorf("Expected no error, but got %q", err)
		}

		if len(tc.minimal.Answer) != len(rec.Msg.Answer) {
			t.Errorf("Test %d: Expected %d Answer, but got %d", i, len(tc.minimal.Answer), len(req.Answer))
			continue
		}
		if len(tc.minimal.Ns) != len(rec.Msg.Ns) {
			t.Errorf("Test %d: Expected %d Ns, but got %d", i, len(tc.minimal.Ns), len(req.Ns))
			continue
		}

		if len(tc.minimal.Extra) != len(rec.Msg.Extra) {
			t.Errorf("Test %d: Expected %d Extras, but got %d", i, len(tc.minimal.Extra), len(req.Extra))
			continue
		}

		for j, a := range rec.Msg.Answer {
			if tc.minimal.Answer[j].String() != a.String() {
				t.Errorf("Test %d: Expected Answer %d to be %v, but got %v", i, j, tc.minimal.Answer[j], a)
			}
		}

		for j, a := range rec.Msg.Ns {
			if tc.minimal.Ns[j].String() != a.String() {
				t.Errorf("Test %d: Expected NS %d to be %v, but got %v", i, j, tc.minimal.Ns[j], a)
			}
		}

		for j, a := range rec.Msg.Extra {
			if tc.minimal.Extra[j].String() != a.String() {
				t.Errorf("Test %d: Expected Extra %d to be %v, but got %v", i, j, tc.minimal.Extra[j], a)
			}
		}
	}
}
