package autopath

import (
	"testing"

	"github.com/coredns/coredns/middleware/test"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func TestAutoPath(t *testing.T) {}

var nextMap = map[string]int{}

// nextHandler returns a Handler that returns an answer for the question in the
// request per the domain->answer map.
func nextHandler(mm map[string]int) test.Handler {
	return test.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		rcode, ok := mm[r.Question[0].Name]
		if !ok {
			return dns.RcodeServerFailure, nil
		}

		m := new(dns.Msg)
		m.SetReply(r)

		switch rcode {
		case dns.RcodeNameError:
			r.Rcode = rcode
			m.Ns = []dns.RR{soa}
			w.WriteMsg(m)
			return r.Rcode, nil

		case dns.RcodeSuccess:
			a, _ := dns.NewRR(r.Question[0].Name + " 3600 IN A 127.0.0.53")
			m.Answer = []dns.RR{a}

			w.WriteMsg(m)
			return r.Rcode, nil
		}

		return dns.RcodeServerFailure, nil
	})
}

var soa = func() dns.RR {
	s, _ := dns.NewRR("example.org.		1800	IN	SOA	example.org. example.org. 1502165581 14400 3600 604800 14400")
	return s
}()

func TestInSearchPath(t *testing.T) {
	a := AutoPath{search: []string{"default.svc.cluster.local.", "svc.cluster.local.", "cluster.local."}}

	tests := []struct {
		qname string
		b     bool
	}{
		{"google.com", false},
		{"default.svc.cluster.local.", true},
		{"a.default.svc.cluster.local.", true},
		{"a.b.svc.cluster.local.", false},
	}
	for i, tc := range tests {
		got := a.FirstInSearchPath(tc.qname)
		if got != tc.b {
			t.Errorf("Test %d, got %d, expected %d", i, got, tc.b)
		}
	}
}
