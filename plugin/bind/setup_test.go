package bind

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

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

func TestSetupTentativeAddressIgnored(t *testing.T) {
	origNetInterfaces := netInterfaces
	origGetInterfaceAddrs := getInterfaceAddrs
	origCheckBind := checkBind
	t.Cleanup(func() {
		netInterfaces = origNetInterfaces
		getInterfaceAddrs = origGetInterfaceAddrs
		checkBind = origCheckBind
	})

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("172.18.137.65"), Mask: net.CIDRMask(32, 32)},
			&net.IPNet{IP: net.ParseIP("fe80::6446:60ff:fe2d:49d9"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	checkBind = func(addr string) error {
		if strings.HasPrefix(addr, "fe80::") {
			return syscall.EADDRNOTAVAIL
		}
		return nil
	}

	buf := &bytes.Buffer{}
	clog.SetOutput(buf)
	t.Cleanup(func() {
		clog.Discard()
	})

	c := caddy.NewTestController("dns", "bind eth0")
	err := setup(c)
	if err != nil {
		t.Fatalf("expected no errors, got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	expected := []string{"172.18.137.65"}
	if !slices.Equal(cfg.ListenHosts, expected) {
		t.Errorf("expected ListenHosts %v, got %v", expected, cfg.ListenHosts)
	}

	if !strings.Contains(buf.String(), "cannot assign requested address fe80::6446:60ff:fe2d:49d9%eth0 on interface eth0") {
		t.Errorf("expected warning in log output, got: %q", buf.String())
	}
}

func TestSetupMultipleValidAddressesWithTentative(t *testing.T) {
	origNetInterfaces := netInterfaces
	origGetInterfaceAddrs := getInterfaceAddrs
	origCheckBind := checkBind
	t.Cleanup(func() {
		netInterfaces = origNetInterfaces
		getInterfaceAddrs = origGetInterfaceAddrs
		checkBind = origCheckBind
	})

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(24, 32)},
			&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	checkBind = func(addr string) error {
		if strings.HasPrefix(addr, "fe80::") {
			return syscall.EADDRNOTAVAIL
		}
		return nil
	}

	c := caddy.NewTestController("dns", "bind eth0")
	err := setup(c)
	if err != nil {
		t.Fatalf("expected no errors, got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	expected := []string{"192.0.2.1", "2001:db8::1"}
	if !slices.Equal(cfg.ListenHosts, expected) {
		t.Errorf("expected ListenHosts %v, got %v", expected, cfg.ListenHosts)
	}
}

func TestSetupAllInterfaceAddressesUnavailable(t *testing.T) {
	origNetInterfaces := netInterfaces
	origGetInterfaceAddrs := getInterfaceAddrs
	origCheckBind := checkBind
	t.Cleanup(func() {
		netInterfaces = origNetInterfaces
		getInterfaceAddrs = origGetInterfaceAddrs
		checkBind = origCheckBind
	})

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	checkBind = func(addr string) error {
		return syscall.EADDRNOTAVAIL
	}

	c := caddy.NewTestController("dns", "bind eth0")
	err := setup(c)
	if err == nil {
		t.Fatal("expected setup error when all interface addresses are unavailable, got nil")
	}
	if !strings.Contains(err.Error(), `failed to bind to any address on interface "eth0"`) {
		t.Errorf("expected error message to mention interface failure, got: %v", err)
	}
}

func TestSetupFatalBindError(t *testing.T) {
	origNetInterfaces := netInterfaces
	origGetInterfaceAddrs := getInterfaceAddrs
	origCheckBind := checkBind
	t.Cleanup(func() {
		netInterfaces = origNetInterfaces
		getInterfaceAddrs = origGetInterfaceAddrs
		checkBind = origCheckBind
	})

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}
	checkBind = func(addr string) error {
		return syscall.EACCES
	}

	c := caddy.NewTestController("dns", "bind eth0")
	err := setup(c)
	if err == nil {
		t.Fatal("expected setup error on EACCES, got nil")
	}
	if !strings.Contains(err.Error(), "failed to bind interface address") {
		t.Errorf("expected fatal bind error, got: %v", err)
	}
}

func TestSetupExceptWithNonAssignableAddress(t *testing.T) {
	origNetInterfaces := netInterfaces
	origGetInterfaceAddrs := getInterfaceAddrs
	origCheckBind := checkBind
	t.Cleanup(func() {
		netInterfaces = origNetInterfaces
		getInterfaceAddrs = origGetInterfaceAddrs
		checkBind = origCheckBind
	})

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(24, 32)},
			&net.IPNet{IP: net.ParseIP("192.0.2.2"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}
	checkBind = func(addr string) error {
		return nil
	}

	c := caddy.NewTestController("dns", "bind eth0 {\nexcept 192.0.2.2\n}\n")
	err := setup(c)
	if err != nil {
		t.Fatalf("expected no errors, got: %v", err)
	}

	cfg := dnsserver.GetConfig(c)
	expected := []string{"192.0.2.1"}
	if !slices.Equal(cfg.ListenHosts, expected) {
		t.Errorf("expected ListenHosts %v, got %v", expected, cfg.ListenHosts)
	}
}

func TestIsAddrNotAvailable(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "direct EADDRNOTAVAIL",
			err:      syscall.EADDRNOTAVAIL,
			expected: true,
		},
		{
			name:     "wrapped EADDRNOTAVAIL",
			err:      fmt.Errorf("wrap: %w", syscall.EADDRNOTAVAIL),
			expected: true,
		},
		{
			name: "net.OpError with os.SyscallError EADDRNOTAVAIL",
			err: &net.OpError{
				Op:  "listen",
				Net: "tcp",
				Err: &os.SyscallError{
					Syscall: "bind",
					Err:     syscall.EADDRNOTAVAIL,
				},
			},
			expected: true,
		},
		{
			name:     "cannot assign requested address message",
			err:      errors.New("listen tcp 198.51.100.1:0: bind: cannot assign requested address"),
			expected: true,
		},
		{
			name:     "address not available message",
			err:      errors.New("bind: address not available"),
			expected: true,
		},
		{
			name:     "windows not valid in its context message",
			err:      errors.New("bind: The requested address is not valid in its context"),
			expected: true,
		},
		{
			name:     "unrelated syscall error",
			err:      syscall.EINVAL,
			expected: false,
		},
		{
			name:     "address in use",
			err:      syscall.EADDRINUSE,
			expected: false,
		},
		{
			name:     "permission denied",
			err:      syscall.EACCES,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAddrNotAvailable(tc.err)
			if got != tc.expected {
				t.Errorf("isAddrNotAvailable(%v) = %v, expected %v", tc.err, got, tc.expected)
			}
		})
	}
}

func TestDefaultCheckBind(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping on %s", runtime.GOOS)
	}

	// 127.0.0.1 is available on loopback
	if err := defaultCheckBind("127.0.0.1"); err != nil {
		t.Errorf("expected defaultCheckBind(127.0.0.1) to succeed, got: %v", err)
	}

	// 198.51.100.1 (RFC 5737 TEST-NET-2) is not assigned to any local interface
	err := defaultCheckBind("198.51.100.1")
	if err == nil {
		t.Fatal("expected defaultCheckBind(198.51.100.1) to fail, got nil")
	}
	if !isAddrNotAvailable(err) {
		t.Errorf("expected isAddrNotAvailable to be true for unassigned address error, got: %v", err)
	}
}
