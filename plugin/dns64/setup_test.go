package dns64

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupDns64(t *testing.T) {
	tests := []struct {
		inputUpstreams string
		shouldErr      bool
		prefix         string
	}{
		{
			`dns64`,
			false,
			"64:ff9b::/96",
		},
		{
			`dns64 64:dead::/96`,
			false,
			"64:dead::/96",
		},
		{
			`dns64 {
				translate_all
			}`,
			false,
			"64:ff9b::/96",
		},
		{
			`dns64`,
			false,
			"64:ff9b::/96",
		},
		{
			`dns64 {
				prefix 64:ff9b::/96
			}`,
			false,
			"64:ff9b::/96",
		},
		{
			`dns64 {
				prefix 64:ff9b::/32
			}`,
			false,
			"64:ff9b::/32",
		},
		{
			`dns64 {
				prefix 64:ff9b::/52
			}`,
			true,
			"64:ff9b::/52",
		},
		{
			`dns64 {
				prefix 64:ff9b::/104
			}`,
			true,
			"64:ff9b::/104",
		},
		{
			`dns64 {
				prefix 8.8.8.8/24
			}`,
			true,
			"8.8.9.9/24",
		},
		{
			`dns64 {
				prefix 64:ff9b::/96
			}`,
			false,
			"64:ff9b::/96",
		},
		{
			`dns64 {
				prefix 2002:ac12:b083::/96
			}`,
			false,
			"2002:ac12:b083::/96",
		},
		{
			`dns64 {
				prefix 2002:c0a8:a88a::/48
			}`,
			false,
			"2002:c0a8:a88a::/48",
		},
		{
			`dns64 foobar {
				prefix 64:ff9b::/96
			}`,
			true,
			"64:ff9b::/96",
		},
		{
			`dns64 foobar`,
			true,
			"64:ff9b::/96",
		},
		{
			`dns64 {
				foobar
			}`,
			true,
			"64:ff9b::/96",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputUpstreams)
		dns64, err := dns64Parse(c)
		if (err != nil) != test.shouldErr {
			t.Errorf("Test %d expected %v error, got %v for %s", i+1, test.shouldErr, err, test.inputUpstreams)
		}
		if err == nil {
			if dns64.Prefix.String() != test.prefix {
				t.Errorf("Test %d expected prefix %s, got %v", i+1, test.prefix, dns64.Prefix.String())
			}
		}
	}
}
