package fallback

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetupFallback(t *testing.T) {
	c := caddy.NewTestController("dns", `fallback`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	c = caddy.NewTestController("dns", `fallback example.org {
    on NXDOMAIN 10.10.10.10:100 8.8.8.8:53
}`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
}
