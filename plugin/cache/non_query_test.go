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
