package forward

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"testing"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*Proxy{{addr: "1.1.1.1:53"}, {addr: "2.2.2.2:53"}, {addr: "3.3.3.3:53"}},
		p:       &roundRobin{},
	}

	expect := []*Proxy{{addr: "2.2.2.2:53"}, {addr: "1.1.1.1:53"}, {addr: "3.3.3.3:53"}}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.addr != expect[i].addr {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].addr, p.addr)
		}
	}
}

func TestSetTapPlugin(t *testing.T) {
	input := `
      dnstap /tmp/dnstap.sock full
      dnstap tcp://example.com:6000
    `
	c := caddy.NewTestController("dns", input)
	dnstapSetup, err := caddy.DirectiveAction("dns", "dnstap")
	if err != nil {
		t.Fatal(err)
	}
	if err = dnstapSetup(c); err != nil {
		t.Fatal(err)
	}
	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})

	if taph := dnsserver.GetConfig(c).Handler("dnstap"); taph != nil {
		f := New()
		f.SetTapPlugin(taph)
		if len(f.tapPlugins) != 2 {
			t.Fatalf("Expected: 2 results, got: %v", len(f.tapPlugins))
		}
		if f.tapPlugins[0] != taph || f.tapPlugins[0].Next != f.tapPlugins[1] {
			t.Fatal("Unexpected order of dnstap plugins")
		}
	} else {
		t.Error("Expected first plugin to be dnstap")
	}
}
