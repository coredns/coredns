package autopath

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// TestAutoPathLeavesNextAnswerAlone checks that the CNAME autopath prepends does not
// write into the answer slice the next plugin handed over. A backend may answer with
// a slice it owns: plugin/file answers straight out of the zone tree, and an RRset
// built with append can have spare capacity.
func TestAutoPathLeavesNextAnswerAlone(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{name: "spare capacity", capacity: 4},
		// Control: with no spare capacity append has to reallocate, so the owner's
		// slice is safe even without care in cnamer.
		{name: "no spare capacity", capacity: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owned := make([]dns.RR, 3, tc.capacity)
			owned[0] = test.A("x.b.example.org. 3600 IN A 127.0.0.1")
			owned[1] = test.A("x.b.example.org. 3600 IN A 127.0.0.2")
			owned[2] = test.A("x.b.example.org. 3600 IN A 127.0.0.3")
			want := append([]dns.RR(nil), owned...)

			ap := new(AutoPath)
			ap.Zones = []string{"."}
			ap.search = []string{"a.example.org.", "b.example.org.", ""}
			ap.Next = test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				m := new(dns.Msg)
				m.SetReply(r)
				if r.Question[0].Name != "x.b.example.org." {
					m.Rcode = dns.RcodeNameError
					m.Ns = []dns.RR{soa}
					w.WriteMsg(m)
					return m.Rcode, nil
				}
				m.Answer = owned
				w.WriteMsg(m)
				return dns.RcodeSuccess, nil
			})

			tcase := test.Case{
				Qname: "x.a.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("x.a.example.org. 3600 IN CNAME x.b.example.org."),
					test.A("x.b.example.org. 3600 IN A 127.0.0.1"),
					test.A("x.b.example.org. 3600 IN A 127.0.0.2"),
					test.A("x.b.example.org. 3600 IN A 127.0.0.3"),
				},
			}

			for i := range 3 {
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				if _, err := ap.ServeDNS(context.TODO(), rec, tcase.Msg()); err != nil {
					t.Fatalf("Query %d: expected no error, got %v", i, err)
				}

				if len(owned) != len(want) {
					t.Fatalf("Query %d: next plugin's answer has %d records, expected %d", i, len(owned), len(want))
				}
				for j := range want {
					if owned[j] != want[j] {
						t.Errorf("Query %d: next plugin's answer[%d] was modified: got %q, expected %q", i, j, owned[j], want[j])
					}
				}

				// No sorting here as we want to check if the CNAME sits *before* the
				// rest of the answer.
				resp := rec.Msg
				if resp == nil {
					t.Fatalf("Query %d: expected a reply, got none", i)
				}
				if err := test.Header(tcase, resp); err != nil {
					t.Errorf("Query %d: %v", i, err)
					continue
				}
				if err := test.Section(tcase, test.Answer, resp.Answer); err != nil {
					t.Errorf("Query %d: %v", i, err)
				}
			}
		})
	}
}
