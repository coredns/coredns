package kubernetes

import (
	"strconv"
	"testing"

	"github.com/mholt/caddy"
)

func TestKubernetesParseReverseZone(t *testing.T) {
	normalizedNonMod8Cidr := []string{"coredns.local."}
	for i := 0 ; i < 128 ; i++ {
		normalizedNonMod8Cidr = append(normalizedNonMod8Cidr, strconv.Itoa(i) + ".0.10.in-addr.arpa.")
	}

	tests := []struct {
		input         string   // Corefile data as string
		expectedZones []string // expected count of defined zones.
	}{
		{`kubernetes coredns.local 10.0.0.0/16`, []string{"coredns.local.", "0.10.in-addr.arpa."}},
		{`kubernetes coredns.local 10.0.0.0/17`, normalizedNonMod8Cidr},
		{`kubernetes coredns.local fd00:77:30::0/110`,
			[]string{"coredns.local.",
				"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.",
				"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.",
				"2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.",
				"3.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.7.7.0.0.0.0.d.f.ip6.arpa.",
			},
		},
	}

	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		k, err := kubernetesParse(c)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, got %q", i, err)
		}

		zl := len(k.Zones)
		if zl != len(tc.expectedZones) {
			t.Errorf("Test %d: Expected kubernetes to be initialized with %d zones, found %d zones", i, len(tc.expectedZones), zl)
		}
		for j, z := range tc.expectedZones {
			if k.Zones[j] != z {
				t.Errorf("Test %d: Expected zones to be %q, got %q", i, z, k.Zones[j])
			}
		}
	}
}
