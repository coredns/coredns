package bind

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/mholt/caddy"
	"testing"
)

func TestSetupBind(t *testing.T) {
	for i, test := range []struct {
		bindToken string
		addresses []string
	}{
		{`bind 1.2.3.4`, []string{"1.2.3.4"}},
		{`bind 1.2.3.4 ::1`, []string{"1.2.3.4", "::1"}},
	} {
		c := caddy.NewTestController("dns", test.bindToken)
		enh, err := setupEnhancerBind(&c.Dispenser)
		if err != nil {
			t.Fatalf("test %d expected no errors, but got: %v", i, err)
		}
		za := dnsserver.ZoneAddr{Transport: "dns", Zone: ".", Port: "53", Options: map[string]string{}}
		zas := enh(za)
		if len(zas) != len(test.addresses) {
			t.Fatalf("test %d: too much ZoneAddr returns, expected %v, got %v", i, len(test.addresses), len(zas))
		}
		for i, addr := range test.addresses {
			if zas[i].ListeningAddr != addr {
				t.Fatalf("test %d: invalid address injected, expected %v, got %v", i, addr, zas[i].ListeningAddr)
			}
		}
	}
}
