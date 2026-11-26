package bind

import (
	"runtime"
	"slices"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestListIPAddresses(t *testing.T) {
	for _, test := range []struct {
		address string
		failing bool
	}{
		{"127.0.0.1", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"::", false},
		{"::ffff:127.0.0.1", false},
		{"fe80::1%eth0", false},
		{"fe80::1%2", false},
		{"FE80:0:0:0:0:0:0:1%eth0", false},
		{"fe80::1%Local Area Connection", false},
		{"", true},
		{"localhost", true},
		{"localhost.", true},
		{"not-an-interface.invalid", true},
		{"fe80::1%", true},
		{"%eth0", true},
		{"127.0.0.1%eth0", true},
		{"[fe80::1%eth0]", true},
		{"[fe80::1%eth0]:53", true},
		{"127.0.0.1:53", true},
		{"fe80::1/64", true},
	} {
		t.Run(test.address, func(t *testing.T) {
			// No interfaces or DNS lookup should be needed to validate a literal.
			got, err := listIP([]string{test.address}, nil)
			if test.failing {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, []string{test.address}) {
				t.Errorf("expected the address and zone to be preserved, got %q", got)
			}
		})
	}
}

func TestSetupScopedAddresses(t *testing.T) {
	for _, test := range []struct {
		name     string
		config   string
		expected []string
		failing  bool
	}{
		{"named zone", `bind fe80::1%eth0`, []string{"fe80::1%eth0"}, false},
		{"numeric zone", `bind fe80::1%2`, []string{"fe80::1%2"}, false},
		{"quoted zone", `bind "fe80::1%Local Area Connection"`, []string{"fe80::1%Local Area Connection"}, false},
		{"exclude one zone", "bind fe80::1%eth0 fe80::1%eth1 {\nexcept fe80::1%eth0\n}", []string{"fe80::1%eth1"}, false},
		{"exclude numeric zone", "bind fe80::1%2 127.0.0.1 {\nexcept fe80::1%2\n}", []string{"127.0.0.1"}, false},
		{"empty address", `bind ""`, nil, true},
		{"empty exclusion", "bind 127.0.0.1 {\nexcept \"\"\n}", nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", test.config)
			err := setup(c)
			if test.failing {
				if err == nil {
					t.Fatal("expected setup to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := dnsserver.GetConfig(c).ListenHosts; !slices.Equal(got, test.expected) {
				t.Errorf("ListenHosts = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestSetup(t *testing.T) {
	// Skip on non-Linux systems as some tests refer to for e.g. loopback interfaces which
	// are not present on all systems.
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping bind test on %s", runtime.GOOS)
	}

	for i, test := range []struct {
		config   string
		expected []string
		failing  bool
	}{
		{`bind 1.2.3.4`, []string{"1.2.3.4"}, false},
		{`bind`, nil, true},
		{`bind 1.2.3.invalid`, nil, true},
		{`bind 1.2.3.4 ::5`, []string{"1.2.3.4", "::5"}, false},
		{`bind ::1 1.2.3.4 ::5 127.9.9.0`, []string{"::1", "1.2.3.4", "::5", "127.9.9.0"}, false},
		{`bind ::1 1.2.3.4 ::5 127.9.9.0 noone`, nil, true},
		{`bind 1.2.3.4 lo`, []string{"1.2.3.4", "127.0.0.1", "::1"}, false},
		{"bind lo {\nexcept 127.0.0.1\n}\n", []string{"::1"}, false},
	} {
		c := caddy.NewTestController("dns", test.config)
		err := setup(c)
		if err != nil {
			if !test.failing {
				t.Fatalf("Test %d, expected no errors, but got: %v", i, err)
			}
			continue
		}
		if test.failing {
			t.Fatalf("Test %d, expected to failed but did not, returned values", i)
		}
		cfg := dnsserver.GetConfig(c)
		if len(cfg.ListenHosts) != len(test.expected) {
			t.Errorf("Test %d : expected the config's ListenHosts size to be %d, was %d", i, len(test.expected), len(cfg.ListenHosts))
			continue
		}
		for i, v := range test.expected {
			if got, want := cfg.ListenHosts[i], v; got != want {
				t.Errorf("Test %d : expected the config's ListenHost to be %s, was %s", i, want, got)
			}
		}
	}
}
