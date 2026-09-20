package cache

// Regression characterization from the original investigation of issue #6184.
import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

func TestResearch6184(t *testing.T) {
	for _, tc := range []struct {
		name           string
		do             []bool
		cd             []bool
		calls, entries int
	}{
		{"same_do_positive_control", []bool{false, false}, []bool{false, false}, 1, 1},
		{"do_upgrade_single_entry", []bool{false, true, false, true}, []bool{false, false, false, false}, 2, 1},
		{"do_downgrade_reuses_signed_entry", []bool{true, false}, []bool{false, false}, 1, 1},
		{"cd_isolation_positive_control", []bool{true, true}, []bool{false, true}, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			calls := 0
			c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				calls++
				m := new(dns.Msg)
				m.SetReply(r)
				m.Answer = []dns.RR{test.A("probe.example. 300 IN A 192.0.2.1")}
				if r.IsEdns0() != nil && r.IsEdns0().Do() {
					m.Answer = append(m.Answer, test.RRSIG("probe.example. 300 IN RRSIG A 8 2 300 20300101000000 20200101000000 1 example. AQ=="))
				}
				return dns.RcodeSuccess, w.WriteMsg(m)
			})
			for i, do := range tc.do {
				q := new(dns.Msg)
				q.SetQuestion("probe.example.", dns.TypeA)
				q.SetEdns0(1232, do)
				q.CheckingDisabled = tc.cd[i]
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				if _, err := c.ServeDNS(context.Background(), rec, q); err != nil {
					t.Fatal(err)
				}
				sigs := 0
				for _, rr := range rec.Msg.Answer {
					if rr.Header().Rrtype == dns.TypeRRSIG {
						sigs++
					}
				}
				if (sigs > 0) != do {
					t.Errorf("query %d DO=%v signatures=%d", i, do, sigs)
				}
			}
			t.Logf("backend calls=%d cache entries=%d", calls, c.pcache.Len())
			if calls != tc.calls {
				t.Errorf("backend calls=%d want %d", calls, tc.calls)
			}
			if c.pcache.Len() != tc.entries {
				t.Errorf("cache entries=%d want %d", c.pcache.Len(), tc.entries)
			}
		})
	}
}
