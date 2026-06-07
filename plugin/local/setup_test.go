package local

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
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
