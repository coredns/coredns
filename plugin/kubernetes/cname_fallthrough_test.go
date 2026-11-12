package kubernetes

import (
	"context"
	"testing"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type cnameFallthroughController struct {
	APIConnServeTest
	services map[string][]*object.Service
}

func (c cnameFallthroughController) SvcIndex(key string) []*object.Service {
	if services, ok := c.services[key]; ok {
		return services
	}
	return c.APIConnServeTest.SvcIndex(key)
}

func TestExternalNameTargetFallthrough(t *testing.T) {
	const alias = "alias.testns.svc.cluster.local."
	const target = "missing.testns.svc.cluster.local."
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		for _, tc := range []struct {
			name        string
			fall        []string
			chain       bool
			wantForward bool
			wantAnswers int
			existing    bool
			nodata      bool
			loop        bool
			truncated   bool
		}{
			{name: "disabled"},
			{name: "all", fall: []string{"."}, wantForward: true, wantAnswers: 2},
			{name: "target zone", fall: []string{target}, wantForward: true, wantAnswers: 2},
			{name: "alias only", fall: []string{alias}},
			{name: "unrelated zone", fall: []string{"in-addr.arpa."}},
			{name: "chain", fall: []string{target}, chain: true, wantForward: true, wantAnswers: 3},
			{name: "existing target", fall: []string{"."}, existing: true, wantAnswers: 2},
			{name: "existing NODATA", fall: []string{"."}, existing: true, nodata: true},
			{name: "loop", fall: []string{"."}, loop: true},
			{name: "truncated lookup", fall: []string{target}, wantForward: true, wantAnswers: 2, truncated: true},
		} {
			t.Run(dns.TypeToString[qtype]+"/"+tc.name, func(t *testing.T) {
				k := New([]string{"cluster.local."})
				services := map[string][]*object.Service{
					"alias.testns": {cnameFallthroughService(t, "alias", target)},
				}
				if tc.chain {
					services["alias.testns"][0].ExternalName = "middle.testns.svc.cluster.local."
					services["middle.testns"] = []*object.Service{cnameFallthroughService(t, "middle", target)}
				}
				if tc.loop {
					services["alias.testns"][0].ExternalName = alias
				}
				if tc.existing {
					ips := []string{"192.0.2.1", "2001:db8::1"}
					if tc.nodata {
						ips = []string{"192.0.2.1"}
						if qtype == dns.TypeA {
							ips = []string{"2001:db8::1"}
						}
					}
					services["missing.testns"] = []*object.Service{cnameFallthroughService(t, "missing", "", ips...)}
				}
				k.APIConn = cnameFallthroughController{services: services}
				k.Upstream = upstream.New()
				if tc.fall != nil {
					k.Fall.SetZonesFromArgs(tc.fall)
				}
				calls := 0
				terminal := plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					calls++
					if q := r.Question[0]; q.Name != target || q.Qtype != qtype {
						t.Fatalf("forwarded question = %v, want %s %s", q, target, dns.TypeToString[qtype])
					}
					m := new(dns.Msg).SetReply(r)
					m.Truncated = tc.truncated
					if qtype == dns.TypeA {
						m.Answer = []dns.RR{test.A(target + " 30 IN A 192.0.2.1")}
					} else {
						m.Answer = []dns.RR{test.AAAA(target + " 30 IN AAAA 2001:db8::1")}
					}
					return dns.RcodeSuccess, w.WriteMsg(m)
				})
				config := &dnsserver.Config{Zone: "."}
				config.AddPlugin(func(next plugin.Handler) plugin.Handler { k.Next = next; return k })
				config.AddPlugin(func(plugin.Handler) plugin.Handler { return terminal })
				server, err := dnsserver.NewServer("dns://:0", []*dnsserver.Config{config})
				if err != nil {
					t.Fatal(err)
				}
				r := new(dns.Msg).SetQuestion(alias, qtype)
				w := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := context.WithValue(t.Context(), dnsserver.Key{}, server)
				server.ServeDNS(ctx, w, r)
				if w.Msg == nil {
					t.Fatal("no response")
				}
				if w.Msg.Rcode != dns.RcodeSuccess {
					t.Fatalf("rcode = %s", dns.RcodeToString[w.Msg.Rcode])
				}
				wantCalls := 0
				if tc.wantForward {
					wantCalls = 1
				}
				if calls != wantCalls || len(w.Msg.Answer) != tc.wantAnswers {
					t.Fatalf("forward calls = %d, answers = %d; want %d, %d; response: %s", calls, len(w.Msg.Answer), wantCalls, tc.wantAnswers, w.Msg)
				}
				if w.Msg.Truncated != tc.truncated {
					t.Fatalf("truncated = %v, want %v", w.Msg.Truncated, tc.truncated)
				}
				if tc.wantAnswers > 0 {
					if w.Msg.Answer[0].Header().Name != alias {
						t.Fatal("missing alias record")
					}
					last := w.Msg.Answer[len(w.Msg.Answer)-1]
					if last.Header().Name != target || last.Header().Rrtype != qtype {
						t.Fatalf("unexpected target answer: %s", last)
					}
				}
				if err := test.CNAMEOrder(w.Msg); err != nil {
					t.Fatal(err)
				}
				if w.Msg.Question[0] != r.Question[0] {
					t.Fatal("original question changed")
				}
			})
		}
	}
}

func cnameFallthroughService(t *testing.T, name, target string, ips ...string) *object.Service {
	t.Helper()
	typ := api.ServiceTypeClusterIP
	if target != "" {
		typ = api.ServiceTypeExternalName
	}
	svc, err := object.ToService(&api.Service{
		ObjectMeta: meta.ObjectMeta{Name: name, Namespace: "testns"},
		Spec:       api.ServiceSpec{Type: typ, ExternalName: target, ClusterIPs: ips},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc.(*object.Service)
}
