package siit

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupSiit(t *testing.T) {
	tests := []struct {
		inputUpstreams string
		shouldErr      bool
		wantIPv6Prefix string
		wantEam        map[string]string
	}{
		{
			`siit`,
			false,
			"64:ff9b::/96",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:dead::/96
			}`,
			false,
			"64:dead::/96",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 10.0.0.0/8
			}`,
			true,
			"10.0.0.0/8",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix foobar
			}`,
			true,
			"foobar",
			map[string]string{},
		},
		{
			`siit {
				eam 10.0.0.1 64:dead::1
			}`,
			false,
			"64:ff9b::/96",
			map[string]string{
				"10.0.0.1": "64:dead::1",
			},
		},
		{
			`siit {
				eam 10.0.0.1 64:dead::1
				eam 10.0.0.2 64:dead::2
			}`,
			false,
			"64:ff9b::/96",
			map[string]string{
				"10.0.0.1": "64:dead::1",
				"10.0.0.2": "64:dead::2",
			},
		},
		{
			`siit {
				eam 64:dead::1 10.0.0.1
			}`,
			true,
			"64:ff9b::/96",
			map[string]string{
				"64:dead::1": "10.0.0.1",
			},
		},
		{
			`siit {
				eam foobar 64:dead::1
			}`,
			true,
			"64:ff9b::/96",
			map[string]string{
				"foobar": "64:dead::1",
			},
		},
		{
			`siit {
				eam 10.0.0.1 foobar
			}`,
			true,
			"64:ff9b::/96",
			map[string]string{
				"10.0.0.1": "foobar",
			},
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputUpstreams)
		siit, err := siitParse(c)
		if (err != nil) != test.shouldErr {
			t.Errorf("Test %d expected %v error, got %v for %s", i+1, test.shouldErr, err, test.inputUpstreams)
		}
		if err == nil {
			if siit.IPv6Prefix.String() != test.wantIPv6Prefix {
				t.Errorf("Test %d expected ipv6 prefix %s, got %v", i+1, test.wantIPv6Prefix, siit.IPv6Prefix.String())
			}
		}
	}
}
