package whoareyou

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetupWhoareyou(t *testing.T) {
	c := caddy.NewTestController("dns", `whoareyou`)
	if err := setupWhoareyou(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	c = caddy.NewTestController("dns", `whoareyou example.org`)
	if err := setupWhoareyou(c); err == nil {
		t.Fatalf("Expected errors, but got: %v", err)
	}
}
