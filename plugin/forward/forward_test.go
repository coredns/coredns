package forward

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*proxy.Proxy{
			proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
			proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
			proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
		},
		p: &roundRobin{},
	}

	expect := []*proxy.Proxy{
		proxy.NewProxy("TestList", "2.2.2.2:53", transport.DNS),
		proxy.NewProxy("TestList", "1.1.1.1:53", transport.DNS),
		proxy.NewProxy("TestList", "3.3.3.3:53", transport.DNS),
	}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.Addr() != expect[i].Addr() {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].Addr(), p.Addr())
		}
	}
}

func TestSetTapPlugin(t *testing.T) {
	input := `forward . 127.0.0.1
	dnstap /tmp/dnstap.sock full
	dnstap tcp://example.com:6000
	`
	stanzas := strings.Split(input, "\n")
	c := caddy.NewTestController("dns", strings.Join(stanzas[1:], "\n"))
	dnstapSetup, err := caddy.DirectiveAction("dns", "dnstap")
	if err != nil {
		t.Fatal(err)
	}
	if err = dnstapSetup(c); err != nil {
		t.Fatal(err)
	}
	c.Dispenser = caddyfile.NewDispenser("", strings.NewReader(stanzas[0]))
	if err = setup(c); err != nil {
		t.Fatal(err)
	}
	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})
	f, ok := dnsserver.GetConfig(c).Handler("forward").(*Forward)
	if !ok {
		t.Fatal("Expected a forward plugin")
	}
	tap, ok := dnsserver.GetConfig(c).Handler("dnstap").(*dnstap.Dnstap)
	if !ok {
		t.Fatal("Expected a dnstap plugin")
	}
	f.SetTapPlugin(tap)
	if len(f.tapPlugins) != 2 {
		t.Fatalf("Expected: 2 results, got: %v", len(f.tapPlugins))
	}
	if f.tapPlugins[0] != tap || tap.Next != f.tapPlugins[1] {
		t.Error("Unexpected order of dnstap plugins")
	}
}
