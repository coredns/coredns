//go:build coredns_all || coredns_any

package any

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `any`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	// Check that the plugin was added to the config
	cfg := dnsserver.GetConfig(c)
	if len(cfg.Plugin) == 0 {
		t.Error("Expected plugin to be added to config")
	}
}
