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
)

type apiConnExternalNameLoops struct {
	APIConnServeTest
	svcs map[string][]*object.Service
}

func (a apiConnExternalNameLoops) SvcIndex(s string) []*object.Service { return a.svcs[s] }
func (a apiConnExternalNameLoops) ServiceList() []*object.Service {
	var all []*object.Service
	for _, s := range a.svcs {
		all = append(all, s...)
	}
	return all
}
func (apiConnExternalNameLoops) EpIndex(string) []*object.Endpoints { return nil }
func (apiConnExternalNameLoops) EndpointsList() []*object.Endpoints { return nil }

func externalNameService(name, namespace, target string) *object.Service {
	return &object.Service{
		Name:         name,
		Namespace:    namespace,
		Type:         api.ServiceTypeExternalName,
		ExternalName: target,
	}
}

func TestExternalNameLoopsAreBounded(t *testing.T) {
	conn := apiConnExternalNameLoops{svcs: map[string][]*object.Service{
		// Two-cycle across zones.
		"a.ns1": {externalNameService("a", "ns1", "b.ns2.svc.other.local.")},
		"b.ns2": {externalNameService("b", "ns2", "a.ns1.svc.cluster.local.")},
		// Direct self-reference and a reference to a name below itself.
		"c.ns1": {externalNameService("c", "ns1", "c.ns1.svc.cluster.local.")},
		"d.ns1": {externalNameService("d", "ns1", "sub.d.ns1.svc.cluster.local.")},
		// Three-cycle alternating between the zones.
		"e.ns1": {externalNameService("e", "ns1", "f.ns2.svc.other.local.")},
		"f.ns2": {externalNameService("f", "ns2", "g.ns1.svc.cluster.local.")},
		"g.ns1": {externalNameService("g", "ns1", "e.ns1.svc.cluster.local.")},
		// Two-cycle inside one zone (in-zone recursion, no self-lookup).
		"h.ns1": {externalNameService("h", "ns1", "i.ns1.svc.cluster.local.")},
		"i.ns1": {externalNameService("i", "ns1", "h.ns1.svc.cluster.local.")},
	}}

	k := New([]string{"cluster.local.", "other.local."})
	k.APIConn = conn
	k.Upstream = upstream.New()
	k.Next = test.ErrorHandler()

	calls := 0
	counting := plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls++
		return k.ServeDNS(ctx, w, r)
	})
	cfg := &dnsserver.Config{Zone: ".", Plugin: []plugin.Plugin{func(plugin.Handler) plugin.Handler { return counting }}}
	server, err := dnsserver.NewServer("dns://:53", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx := context.WithValue(context.Background(), dnsserver.Key{}, server)

	cases := []struct {
		qname     string
		wantCalls int
	}{
		// a -> b (self-lookup) -> a: a is an ancestor, stop before another self-lookup.
		{"a.ns1.svc.cluster.local.", 2},
		{"b.ns2.svc.other.local.", 2},
		// x CNAME x and x CNAME y.x never leave the first call.
		{"c.ns1.svc.cluster.local.", 1},
		{"d.ns1.svc.cluster.local.", 1},
		// e -> f (self-lookup) -> g (self-lookup) -> e: g and e share a zone, so
		// the last hop is in-zone recursion and finds f already chased.
		{"e.ns1.svc.cluster.local.", 3},
		// h <-> i stays inside the zone and never re-enters the server.
		{"h.ns1.svc.cluster.local.", 1},
	}

	// TXT is not exercised: the kubernetes plugin answers TXT only for
	// dns-version.<zone> and never returns an ExternalName target for it.
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		for _, tc := range cases {
			calls = 0
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, qtype)
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			server.ServeDNS(ctx, w, m)

			if w.Msg == nil {
				t.Fatalf("%s %s: no response written", tc.qname, dns.TypeToString[qtype])
			}
			if calls != tc.wantCalls {
				t.Errorf("%s %s: expected %d entries into the plugin chain, got %d", tc.qname, dns.TypeToString[qtype], tc.wantCalls, calls)
			}
			// A cycle yields at most one CNAME per distinct name in it.
			if len(w.Msg.Answer) > 3 {
				t.Errorf("%s %s: expected a short answer, got %d records: %v", tc.qname, dns.TypeToString[qtype], len(w.Msg.Answer), w.Msg.Answer)
			}
			for _, rr := range w.Msg.Answer {
				if rr.Header().Rrtype != dns.TypeCNAME {
					t.Errorf("%s %s: unexpected non-CNAME record in a cyclic answer: %v", tc.qname, dns.TypeToString[qtype], rr)
				}
			}
		}
	}
}
