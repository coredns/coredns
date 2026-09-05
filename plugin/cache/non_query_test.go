package cache

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestCachePassesNonQueryToNext(t *testing.T) {
	c := New()
	called := false
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		called = true
		if r.Opcode != dns.OpcodeUpdate {
			t.Errorf("next handler received opcode %d, want UPDATE", r.Opcode)
		}
		m := new(dns.Msg).SetReply(r)
		if err := w.WriteMsg(m); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})

	r := new(dns.Msg).SetUpdate("example.org.")
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := c.ServeDNS(context.Background(), w, r)
	if err != nil || code != dns.RcodeSuccess {
		t.Fatalf("ServeDNS returned code=%d err=%v", code, err)
	}
	if !called {
		t.Fatal("non-QUERY message did not reach the next handler")
	}
	if w.Msg == nil || w.Msg.Opcode != dns.OpcodeUpdate {
		t.Fatalf("response = %#v, want an UPDATE response", w.Msg)
	}
}

func TestCacheBypassesMutableZones(t *testing.T) {
	c := New()
	calls := 0
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls++
		m := new(dns.Msg).SetReply(r)
		rr, err := dns.NewRR(r.Question[0].Name + " 60 IN A 192.0.2.1")
		if err != nil {
			return dns.RcodeServerFailure, err
		}
		m.Answer = []dns.RR{rr}
		return dns.RcodeSuccess, w.WriteMsg(m)
	})
	query := func(name string) {
		t.Helper()
		r := new(dns.Msg)
		r.SetQuestion(name, dns.TypeA)
		w := dnstest.NewRecorder(&test.ResponseWriter{})
		if code, err := c.ServeDNS(context.Background(), w, r); code != dns.RcodeSuccess || err != nil {
			t.Fatalf("query: %d %v", code, err)
		}
	}
	query("mutable.example.org.")
	c.bypass = []string{"example.org."}
	query("mutable.example.org.")
	query("mutable.example.org.")
	if calls != 3 {
		t.Fatalf("mutable queries used cache: %d upstream calls", calls)
	}
	query("static.example.net.")
	query("static.example.net.")
	if calls != 4 {
		t.Fatalf("unrelated zone was not cached: %d upstream calls", calls)
	}
}
