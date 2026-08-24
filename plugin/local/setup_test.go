package local

import (
	"context"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `local`)
	if err := setup(c); err != nil {
		t.Fatalf("expected no errors, but got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	if len(cfg.Plugin) == 0 {
		t.Fatal("expected plugin to be added to config")
	}
}

func TestSetupRejectsArgs(t *testing.T) {
	c := caddy.NewTestController("dns", `local example.org`)
	if err := setup(c); err == nil {
		t.Fatal("expected error for unexpected argument, got nil")
	}
}

func TestSetupRejectsBlockOptions(t *testing.T) {
	c := caddy.NewTestController("dns", `local { foo }`)
	if err := setup(c); err == nil {
		t.Fatal("expected error for unexpected block option, got nil")
	}
}

func TestSetupLocalhostPrefixOption(t *testing.T) {
	c := caddy.NewTestController("dns", `local {
		localhost_prefix off
	}`)
	if err := setup(c); err != nil {
		t.Fatalf("expected no errors, but got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	if len(cfg.Plugin) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(cfg.Plugin))
	}

	handler := cfg.Plugin[0](plugin.HandlerFunc(func(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
		return 0, nil
	}))
	l, ok := handler.(Local)
	if !ok {
		t.Fatalf("expected Local handler, got %T", handler)
	}
	if !l.disableLocalhostPrefix {
		t.Fatal("expected localhost_prefix off to disable legacy localhost prefix handling")
	}
}

func TestSetupRejectsInvalidLocalhostPrefixValue(t *testing.T) {
	c := caddy.NewTestController("dns", `local {
		localhost_prefix maybe
	}`)
	if err := setup(c); err == nil {
		t.Fatal("expected error for invalid localhost_prefix value, got nil")
	}
}
