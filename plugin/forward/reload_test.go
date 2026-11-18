package forward

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/proxy"
)

func writeResolvConf(t *testing.T, path string, servers ...string) {
	t.Helper()
	var b strings.Builder
	for _, s := range servers {
		b.WriteString("nameserver " + s + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("Failed to write %s: %s", path, err)
	}
}

func proxyAddrs(f *Forward) []string {
	var addrs []string
	for _, p := range f.proxyList() {
		addrs = append(addrs, p.Addr())
	}
	return addrs
}

func proxyByAddr(f *Forward, addr string) *proxy.Proxy {
	for _, p := range f.proxyList() {
		if p.Addr() == addr {
			return p
		}
	}
	return nil
}

func TestSetupReload(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		expected  time.Duration
	}{
		{"forward . 127.0.0.1", false, 0},
		{"forward . 127.0.0.1 {\nreload 10s\n}\n", false, 10 * time.Second},
		{"forward . 127.0.0.1 {\nreload 0\n}\n", false, 0},
		{"forward . 127.0.0.1 {\nreload\n}\n", true, 0},
		{"forward . 127.0.0.1 {\nreload -1s\n}\n", true, 0},
		{"forward . 127.0.0.1 {\nreload often\n}\n", true, 0},
		{"forward . 127.0.0.1 {\nreload 1s 2s\n}\n", true, 0},
	}
	for i, test := range tests {
		fs, err := parseForward(caddy.NewTestController("dns", test.input))
		if test.shouldErr {
			if err == nil {
				t.Errorf("Test %d: expected error for %q", i, test.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Test %d: unexpected error for %q: %s", i, test.input, err)
		}
		if fs[0].reloadInterval != test.expected {
			t.Errorf("Test %d: expected reload interval %s, got %s", i, test.expected, fs[0].reloadInterval)
		}
	}
}

func TestReloadFiles(t *testing.T) {
	resolv := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, resolv, "10.0.0.1", "10.0.0.2")

	input := "forward . " + filepath.ToSlash(resolv) + " 127.0.0.1 {\nreload 1h\n}\n"
	fs, err := parseForward(caddy.NewTestController("dns", input))
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	f := fs[0]
	f.OnStartup()
	defer f.OnShutdown()

	if got, want := proxyAddrs(f), []string{"10.0.0.1:53", "10.0.0.2:53", "127.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}
	kept := proxyByAddr(f, "10.0.0.2:53")
	static := proxyByAddr(f, "127.0.0.1:53")

	// Nothing changed, the proxies must stay the same.
	before := f.proxyList()
	if err := f.reloadFiles(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if !slices.Equal(before, f.proxyList()) {
		t.Errorf("Expected proxies to be unchanged")
	}

	// Replace 10.0.0.1 with 10.0.0.3, 127.0.0.1 is also listed in the file now.
	writeResolvConf(t, resolv, "10.0.0.3", "10.0.0.2", "127.0.0.1")
	if err := f.reloadFiles(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if got, want := proxyAddrs(f), []string{"10.0.0.3:53", "10.0.0.2:53", "127.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}
	if proxyByAddr(f, "10.0.0.2:53") != kept {
		t.Errorf("Expected the proxy for an unchanged upstream to be kept")
	}
	if proxyByAddr(f, "127.0.0.1:53") != static {
		t.Errorf("Expected the proxy for the static upstream to be kept")
	}
	if f.Len() != 3 {
		t.Errorf("Expected 3 upstreams, got %d", f.Len())
	}

	// A file without nameservers leaves the static upstream.
	writeResolvConf(t, resolv)
	if err := f.reloadFiles(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if got, want := proxyAddrs(f), []string{"127.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}

	// A missing file is an error and keeps the current upstreams.
	if err := os.Remove(resolv); err != nil {
		t.Fatal(err)
	}
	if err := f.reloadFiles(); err == nil {
		t.Errorf("Expected an error for a missing file")
	}
	if got, want := proxyAddrs(f), []string{"127.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}
}

func TestReloadFilesKeepsUpstreamsWhenEmpty(t *testing.T) {
	resolv := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, resolv, "10.0.0.1")

	fs, err := parseForward(caddy.NewTestController("dns", "forward . "+filepath.ToSlash(resolv)+" {\nreload 1h\n}\n"))
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	f := fs[0]
	f.OnStartup()
	defer f.OnShutdown()

	writeResolvConf(t, resolv)
	if err := f.reloadFiles(); err == nil {
		t.Errorf("Expected an error when no upstreams are left")
	}
	if got, want := proxyAddrs(f), []string{"10.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}
}

func TestReloadFileEmptyAtStartup(t *testing.T) {
	resolv := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, resolv)

	fs, err := parseForward(caddy.NewTestController("dns", "forward . "+filepath.ToSlash(resolv)+" 127.0.0.1 {\nreload 1h\n}\n"))
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	f := fs[0]
	f.OnStartup()
	defer f.OnShutdown()

	writeResolvConf(t, resolv, "10.0.0.1")
	if err := f.reloadFiles(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if got, want := proxyAddrs(f), []string{"10.0.0.1:53", "127.0.0.1:53"}; !slices.Equal(got, want) {
		t.Fatalf("Expected upstreams %v, got %v", want, got)
	}
}

func TestReloadWatcher(t *testing.T) {
	resolv := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, resolv, "10.0.0.1")

	fs, err := parseForward(caddy.NewTestController("dns", "forward . "+filepath.ToSlash(resolv)+" {\nreload 10ms\n}\n"))
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	f := fs[0]
	f.OnStartup()

	writeResolvConf(t, resolv, "10.0.0.2")
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Equal(proxyAddrs(f), []string{"10.0.0.2:53"}) {
		if time.Now().After(deadline) {
			t.Fatalf("Upstreams were not reloaded, got %v", proxyAddrs(f))
		}
		time.Sleep(10 * time.Millisecond)
	}

	f.OnShutdown()
	f.OnShutdown() // must not panic
}

func TestReloadWithoutFile(t *testing.T) {
	fs, err := parseForward(caddy.NewTestController("dns", "forward . 127.0.0.1 {\nreload 10ms\n}\n"))
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	f := fs[0]
	f.OnStartup()
	defer f.OnShutdown()
	if f.reloadStop != nil {
		t.Errorf("Expected no reload goroutine without a file TO")
	}
}
