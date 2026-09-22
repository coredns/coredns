package loop

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestLoop(t *testing.T) {
	l := New(".")
	l.inc()
	if l.seen() != 1 {
		t.Errorf("Failed to inc loop, expected %d, got %d", 1, l.seen())
	}
}

func TestLoop_NonHINFO(t *testing.T) {
	l := New(".")
	l.Next = test.NextHandler(dns.RcodeSuccess, nil)
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	if rc, _ := l.ServeDNS(context.Background(), w, m); rc != dns.RcodeSuccess {
		t.Fatalf("expected %d, got %d", dns.RcodeSuccess, rc)
	}
}

func TestLoop_Disabled(t *testing.T) {
	l := New(".")
	l.setDisabled()
	l.Next = test.NextHandler(dns.RcodeSuccess, nil)
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeHINFO)
	if rc, _ := l.ServeDNS(context.Background(), w, m); rc != dns.RcodeSuccess {
		t.Fatalf("expected %d, got %d", dns.RcodeSuccess, rc)
	}
}

func TestLoop_ZoneMismatch(t *testing.T) {
	l := New("example.org.")
	l.Next = test.NextHandler(dns.RcodeSuccess, nil)
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion("a.example.com.", dns.TypeHINFO)
	if rc, _ := l.ServeDNS(context.Background(), w, m); rc != dns.RcodeSuccess {
		t.Fatalf("expected %d, got %d", dns.RcodeSuccess, rc)
	}
}

func TestLoop_MatchAndInc(t *testing.T) {
	l := New(".")
	l.Next = test.NextHandler(dns.RcodeSuccess, nil)
	l.qname = "1.2.example.org."
	w := &test.ResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion(l.qname, dns.TypeHINFO)

	if l.seen() != 0 {
		t.Fatalf("expected initial seen 0, got %d", l.seen())
	}
	if _, err := l.ServeDNS(context.Background(), w, m); err != nil {
		t.Fatalf("ServeDNS returned error: %v", err)
	}
	if l.seen() != 1 {
		t.Fatalf("expected seen to be 1 after matching query, got %d", l.seen())
	}
}

func TestLoop_SetAddressAndName(t *testing.T) {
	l := New(".")
	l.setAddress("127.0.0.1:1053")
	if l.address() != "127.0.0.1:1053" {
		t.Fatalf("expected address to be set")
	}
	if l.Name() != "loop" {
		t.Fatalf("expected Name() to be 'loop', got %q", l.Name())
	}
}
