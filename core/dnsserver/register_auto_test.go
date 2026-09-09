//go:build !coredns_manual_registration

package dnsserver_test

import (
	"slices"
	"testing"

	"github.com/coredns/caddy"
)

// Capture this before any test can explicitly call Register.
var dnsRegisteredOnImport = slices.Contains(caddy.ListPlugins()["server_types"], "dns")

func TestAutomaticRegistration(t *testing.T) {
	if !dnsRegisteredOnImport {
		t.Fatal("default builds must register the DNS server type on import")
	}
}
