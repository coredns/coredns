package siit

import (
	"reflect"
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
				"64:dead::1": "10.0.0.1",
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
				"64:dead::1": "10.0.0.1",
				"64:dead::2": "10.0.0.2",
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
		{
			`siit {
				ipv6_prefix 64:ff9b::/72
			}`,
			true, // /72 not in the allowed set (32/40/48/56/64/96)
			"64:ff9b::/72",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:ff9b::/32
			}`,
			false,
			"64:ff9b::/32",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:ff9b::/40
			}`,
			false,
			"64:ff9b::/40",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:ff9b::/48
			}`,
			false,
			"64:ff9b::/48",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:ff9b::/56
			}`,
			false,
			"64:ff9b::/56",
			map[string]string{},
		},
		{
			`siit {
				ipv6_prefix 64:ff9b::/64
			}`,
			false,
			"64:ff9b::/64",
			map[string]string{},
		},
		{
			// /96 prefix with nonzero byte 8 must be rejected
			`siit {
				ipv6_prefix 2001:db8:122:344:ff00::/96
			}`,
			true,
			"2001:db8:122:344:ff00::/96",
			map[string]string{},
		},
		{
			// valid /96 prefix, byte 8 zero -- must still be accepted.
			`siit {
				ipv6_prefix 2001:db8::/96
			}`,
			false,
			"2001:db8::/96",
			map[string]string{},
		},
		{
			// eam missing the second argument must not panic
			`siit {
				eam 10.0.0.1
			}`,
			true,
			"64:ff9b::/96",
			map[string]string{
				"10.0.0.1": "",
			},
		},
		{
			// eam with too many arguments should error, not silently ignore extras
			`siit {
				eam 10.0.0.1 64:dead::1 extra
			}`,
			true,
			"64:ff9b::/96",
			map[string]string{},
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
			gotEam := make(map[string]string, len(siit.Eam4))
			for v6, v4 := range siit.Eam4 {
				gotEam[v6] = v4.String()
			}
			if !reflect.DeepEqual(gotEam, test.wantEam) {
				t.Errorf("Test %d expected eam %v, got %v", i+1, test.wantEam, gotEam)
			}
		}
	}
}
