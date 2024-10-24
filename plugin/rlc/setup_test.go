package rlc

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetRlcPluginConfig(t *testing.T) {
	config := `rlc { 
	ttl 12
	capacity 34
}`
	c := caddy.NewTestController("dns", config)
	rlcSetup, err := caddy.DirectiveAction("dns", "rlc")
	if err != nil {
		t.Fatal(err)
	}
	if err = rlcSetup(c); err != nil {
		t.Fatal(err)
	}

	c.Dispenser = caddyfile.NewDispenser("", strings.NewReader(config))
	if err = setupRlc(c); err != nil {
		t.Fatal(err)
	}

	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})

	rlc, ok := dnsserver.GetConfig(c).Handler("rlc").(*RlcHandler)
	if !ok {
		t.Fatal("Expected a rlc plugin")
	}

	if rlc == nil {
		t.Fatal("Expected a valid rlc plugin")
	}

	if rlc.TTL.Seconds() != 12 {
		t.Fatal("Expected TTL to be 12s")
	}

	if rlc.Capacity != 34 {
		t.Fatal("Expected Capacity to be 34")
	}
	if rlc.RemoteEnabled {
		t.Fatal("Expected remote to be disabled")
	}
	if rlc.UseGroupcache {
		t.Fatal("Expected groupcache to be disabled")
	}
}
