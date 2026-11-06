package upstream

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// selfReferencing answers every query with a CNAME to the query name itself
// and then asks the upstream for that target, the way a backend does for a
// CNAME-typed service whose target leads back to the backend.
type selfReferencing struct {
	up    *Upstream
	calls int
}

func (s *selfReferencing) Name() string { return "selfref" }

func (s *selfReferencing) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	s.calls++
	state := request.Request{W: w, Req: r}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Answer = []dns.RR{test.CNAME(state.QName() + " 5 IN CNAME " + state.QName())}
	if resp, err := s.up.Lookup(ctx, state, state.QName(), state.QType()); err == nil && resp != nil {
		m.Answer = append(m.Answer, resp.Answer...)
	}
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func TestLookupDepthIsBounded(t *testing.T) {
	h := &selfReferencing{up: New()}
	cfg := &dnsserver.Config{Zone: ".", Plugin: []plugin.Plugin{func(plugin.Handler) plugin.Handler { return h }}}
	server, err := dnsserver.NewServer("dns://:53", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, server)
	m := new(dns.Msg)
	m.SetQuestion("x.example.org.", dns.TypeA)
	rec := &test.ResponseWriter{}
	server.ServeDNS(ctx, rec, m)

	// The client query plus maxDepth nested lookups.
	if want := maxDepth + 1; h.calls != want {
		t.Fatalf("expected %d invocations of the plugin chain, got %d", want, h.calls)
	}
}

func TestLookupReturnsErrMaxDepth(t *testing.T) {
	server, err := dnsserver.NewServer("dns://:53", []*dnsserver.Config{{Zone: "."}})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx := context.WithValue(context.Background(), dnsserver.Key{}, server)
	ctx = context.WithValue(ctx, depthKey{}, maxDepth)

	m := new(dns.Msg)
	m.SetQuestion("x.example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	if _, err := New().Lookup(ctx, state, "y.example.org.", dns.TypeA); !errors.Is(err, errMaxDepth) {
		t.Fatalf("expected errMaxDepth, got %v", err)
	}
}

func TestLookupDoesNotConsumeServerLoopDepth(t *testing.T) {
	h := &selfReferencing{up: New()}
	cfg := &dnsserver.Config{Zone: ".", Plugin: []plugin.Plugin{func(plugin.Handler) plugin.Handler { return h }}}
	server, err := dnsserver.NewServer("dns://:53", []*dnsserver.Config{cfg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, server)
	ctx = context.WithValue(ctx, dnsserver.LoopKey{}, maxDepth)

	m := new(dns.Msg)
	m.SetQuestion("x.example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	if _, err := New().Lookup(ctx, state, "y.example.org.", dns.TypeA); err != nil {
		t.Fatalf("Lookup unexpectedly used dnsserver.LoopKey: %v", err)
	}
}

func TestLookupWithoutServer(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("x.example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	if _, err := New().Lookup(context.Background(), state, "y.example.org.", dns.TypeA); err == nil {
		t.Fatal("expected an error without a server in the context")
	}
}
