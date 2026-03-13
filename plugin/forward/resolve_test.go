package forward

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestClassifyToAddrs(t *testing.T) {
	// Create a resolv.conf for file test
	const resolv = "test_resolv.conf"
	if err := os.WriteFile(resolv, []byte("nameserver 10.0.0.1\n"), 0666); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(resolv)

	tests := []struct {
		name          string
		input         []string
		wantIPs       int
		wantHostnames int
		wantErr       bool
		errContains   string
	}{
		{
			name:    "simple IP",
			input:   []string{"127.0.0.1"},
			wantIPs: 1,
		},
		{
			name:    "IP with port",
			input:   []string{"127.0.0.1:8053"},
			wantIPs: 1,
		},
		{
			name:    "IPv6",
			input:   []string{"::1"},
			wantIPs: 1,
		},
		{
			name:    "TLS IP",
			input:   []string{"tls://127.0.0.1"},
			wantIPs: 1,
		},
		{
			name:    "resolv.conf file",
			input:   []string{resolv},
			wantIPs: 1,
		},
		{
			name:          "hostname",
			input:         []string{"dns.example.com"},
			wantHostnames: 1,
		},
		{
			name:          "hostname with port",
			input:         []string{"dns.example.com:5353"},
			wantHostnames: 1,
		},
		{
			name:          "TLS hostname",
			input:         []string{"tls://dns.example.com"},
			wantHostnames: 1,
		},
		{
			name:          "k8s service name",
			input:         []string{"rbldnsd.rbldnsd.svc.cluster.local"},
			wantHostnames: 1,
		},
		{
			name:          "mixed IPs and hostnames",
			input:         []string{"127.0.0.1", "dns.example.com", "10.0.0.1"},
			wantIPs:       2,
			wantHostnames: 1,
		},
		{
			name:        "/dev/null returns file error",
			input:       []string{"/dev/null"},
			wantErr:     true,
			errContains: "no nameservers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ips, hostnames, err := classifyToAddrs(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ips) != tc.wantIPs {
				t.Errorf("expected %d IPs, got %d: %v", tc.wantIPs, len(ips), ips)
			}
			if len(hostnames) != tc.wantHostnames {
				t.Errorf("expected %d hostnames, got %d: %v", tc.wantHostnames, len(hostnames), hostnames)
			}
		})
	}
}

func TestParseAsHostEntry(t *testing.T) {
	tests := []struct {
		input     string
		wantOK    bool
		hostname  string
		port      string
		transport string
		zone      string
	}{
		{"dns.example.com", true, "dns.example.com", "53", transport.DNS, ""},
		{"dns.example.com:5353", true, "dns.example.com", "5353", transport.DNS, ""},
		{"tls://dns.example.com", true, "dns.example.com", "853", transport.TLS, ""},
		{"tls://dns.example.com:8853", true, "dns.example.com", "8853", transport.TLS, ""},
		{"tls://dns.example.com%servername.example.com", true, "dns.example.com", "853", transport.TLS, "servername.example.com"},
		{"rbldnsd.rbldnsd.svc.cluster.local", true, "rbldnsd.rbldnsd.svc.cluster.local", "53", transport.DNS, ""},
		// Should fail for IPs
		{"127.0.0.1", false, "", "", "", ""},
		{"::1", false, "", "", "", ""},
		// Should fail for unsupported transports
		{"https://example.com", false, "", "", "", ""},
		// Should fail for empty
		{"", false, "", "", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			entry, ok := parseAsHostEntry(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if !ok {
				return
			}
			if entry.hostname != tc.hostname {
				t.Errorf("expected hostname=%q, got %q", tc.hostname, entry.hostname)
			}
			if entry.port != tc.port {
				t.Errorf("expected port=%q, got %q", tc.port, entry.port)
			}
			if entry.transport != tc.transport {
				t.Errorf("expected transport=%q, got %q", tc.transport, entry.transport)
			}
			if entry.zone != tc.zone {
				t.Errorf("expected zone=%q, got %q", tc.zone, entry.zone)
			}
		})
	}
}

func TestFormatResolvedAddr(t *testing.T) {
	tests := []struct {
		ip, port, trans, zone string
		expected              string
	}{
		{"10.0.0.1", "53", transport.DNS, "", "10.0.0.1:53"},
		{"10.0.0.1", "853", transport.TLS, "", "tls://10.0.0.1:853"},
		{"10.0.0.1", "853", transport.TLS, "example.com", "tls://10.0.0.1%example.com:853"},
		{"::1", "53", transport.DNS, "", "[::1]:53"},
		{"::1", "853", transport.TLS, "", "tls://[::1]:853"},
		{"::1", "853", transport.TLS, "example.com", "tls://[::1%example.com]:853"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatResolvedAddr(tc.ip, tc.port, tc.trans, tc.zone)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestDnsLookup(t *testing.T) {
	// Start a test DNS server that responds to A queries
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.42"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Use the full server address (IP:port) since the test server uses a random port
	ips, err := dnsLookup("myhost.example.com", []string{s.Addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("expected at least one IP")
	}
	found := false
	for _, ip := range ips {
		if ip == "10.0.0.42" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find 10.0.0.42 in %v", ips)
	}
}

func TestSetupResolver(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldErr   bool
		expectedErr string
		resolverLen int
	}{
		{
			name:        "single resolver IP",
			input:       "forward . 127.0.0.1 {\nresolver 10.96.0.10\n}\n",
			resolverLen: 1,
		},
		{
			name:        "multiple resolver IPs",
			input:       "forward . 127.0.0.1 {\nresolver 10.96.0.10 10.96.0.11\n}\n",
			resolverLen: 2,
		},
		{
			name:        "IPv6 resolver",
			input:       "forward . 127.0.0.1 {\nresolver ::1\n}\n",
			resolverLen: 1,
		},
		{
			name:        "resolver not an IP",
			input:       "forward . 127.0.0.1 {\nresolver dns.example.com\n}\n",
			shouldErr:   true,
			expectedErr: "resolver must be an IP address",
		},
		{
			name:        "resolver no args",
			input:       "forward . 127.0.0.1 {\nresolver\n}\n",
			shouldErr:   true,
			expectedErr: "Wrong argument count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			fs, err := parseForward(c)

			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("expected error to contain %q, got: %v", tc.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			f := fs[0]
			if len(f.resolver) != tc.resolverLen {
				t.Errorf("expected %d resolver(s), got %d: %v", tc.resolverLen, len(f.resolver), f.resolver)
			}
		})
	}
}

func TestSetupResolveInterval(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldErr   bool
		expectedErr string
		expected    time.Duration
	}{
		{
			name:     "default interval",
			input:    "forward . 127.0.0.1\n",
			expected: 30 * time.Second,
		},
		{
			name:     "custom interval",
			input:    "forward . 127.0.0.1 {\nresolve_interval 10s\n}\n",
			expected: 10 * time.Second,
		},
		{
			name:     "1 minute interval",
			input:    "forward . 127.0.0.1 {\nresolve_interval 1m\n}\n",
			expected: 1 * time.Minute,
		},
		{
			name:        "negative interval",
			input:       "forward . 127.0.0.1 {\nresolve_interval -5s\n}\n",
			shouldErr:   true,
			expectedErr: "can't be negative",
		},
		{
			name:        "invalid duration",
			input:       "forward . 127.0.0.1 {\nresolve_interval notaduration\n}\n",
			shouldErr:   true,
			expectedErr: "invalid duration",
		},
		{
			name:        "no args",
			input:       "forward . 127.0.0.1 {\nresolve_interval\n}\n",
			shouldErr:   true,
			expectedErr: "Wrong argument count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			fs, err := parseForward(c)

			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("expected error to contain %q, got: %v", tc.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			f := fs[0]
			if f.resolveInterval != tc.expected {
				t.Errorf("expected resolve_interval %v, got %v", tc.expected, f.resolveInterval)
			}
		})
	}
}

func TestSetupWithHostnameTO(t *testing.T) {
	// Start a test DNS server that resolves "myupstream.example.com" to 10.0.0.42
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA && r.Question[0].Name == "myupstream.example.com." {
			ret.Answer = append(ret.Answer, test.A("myupstream.example.com. IN A 10.0.0.42"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Resolve hostname entries directly using the test server's address (with port)
	entries := []hostEntry{{hostname: "myupstream.example.com", port: "53", transport: "dns"}}
	addrs, err := resolveHostEntries(entries, []string{s.Addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("expected at least one resolved address")
	}
	if addrs[0] != "10.0.0.42:53" {
		t.Errorf("expected resolved addr 10.0.0.42:53, got %s", addrs[0])
	}

	// Now test full integration: manually build the Forward with resolver containing port
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr} // includes port for testing
	f.hostEntries = entries

	resolvedAddrs, err := resolveHostEntries(f.hostEntries, f.resolver)
	if err != nil {
		t.Fatalf("resolution failed: %v", err)
	}

	// Create proxies from resolved addresses
	for _, addr := range resolvedAddrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	if len(f.proxies) == 0 {
		t.Fatal("expected at least one proxy")
	}
	if f.proxies[0].Addr() != "10.0.0.42:53" {
		t.Errorf("expected proxy addr 10.0.0.42:53, got %s", f.proxies[0].Addr())
	}
}

func TestSetupMixedIPAndHostnameTO(t *testing.T) {
	// Start a test DNS server
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			ret.Answer = append(ret.Answer, test.A("myupstream.example.com. IN A 10.0.0.42"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Manually build Forward to test mixed IP + hostname with test server port
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}

	// Classify addresses
	ipAddrs, hostnames, err := classifyToAddrs([]string{"127.0.0.1", "myupstream.example.com"})
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	f.hostEntries = hostnames
	f.staticCount = len(ipAddrs)

	// Resolve hostnames
	resolvedAddrs, err := resolveHostEntries(hostnames, f.resolver)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	// Create proxies
	toHosts := append(ipAddrs, resolvedAddrs...)
	for _, addr := range toHosts {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	// Should have 2 proxies: one static IP + one resolved hostname
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(f.proxies))
	}

	if f.proxies[0].Addr() != "127.0.0.1:53" {
		t.Errorf("expected first proxy 127.0.0.1:53, got %s", f.proxies[0].Addr())
	}
	if f.proxies[1].Addr() != "10.0.0.42:53" {
		t.Errorf("expected second proxy 10.0.0.42:53, got %s", f.proxies[1].Addr())
	}
	if f.staticCount != 1 {
		t.Errorf("expected staticCount 1, got %d", f.staticCount)
	}
}

func TestReResolve(t *testing.T) {
	// Start a test DNS server with a counter to change responses
	callCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		callCount++
		if r.Question[0].Qtype == dns.TypeA {
			// Return different IPs on different calls
			if callCount <= 2 {
				ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.1"))
			} else {
				ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.2"))
			}
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Manually build Forward with test server resolver (includes port)
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.hostEntries = []hostEntry{{hostname: "myhost.example.com", port: "53", transport: "dns"}}
	f.staticCount = 1

	// Create static proxy
	p := proxy.NewProxy("forward", "127.0.0.1:53", "dns")
	f.proxies = append(f.proxies, p)

	// Initial resolution
	resolvedAddrs, err := resolveHostEntries(f.hostEntries, f.resolver)
	if err != nil {
		t.Fatalf("initial resolve error: %v", err)
	}
	for _, addr := range resolvedAddrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		dp := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, dp)
	}

	f.OnStartup()
	defer f.OnShutdown()

	// Initial state: 2 proxies (1 static + 1 dynamic)
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(f.proxies))
	}
	if f.proxies[1].Addr() != "10.0.0.1:53" {
		t.Errorf("expected initial proxy 10.0.0.1:53, got %s", f.proxies[1].Addr())
	}

	// Trigger re-resolution (simulates background timer)
	f.reResolve()

	// After re-resolve, the IP should have changed
	f.proxyMu.RLock()
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies after re-resolve, got %d", len(f.proxies))
	}
	newAddr := f.proxies[1].Addr()
	f.proxyMu.RUnlock()

	if newAddr != "10.0.0.2:53" {
		t.Errorf("expected re-resolved proxy 10.0.0.2:53, got %s", newAddr)
	}
}

func TestReResolveNoChange(t *testing.T) {
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.1"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Manually build Forward with test server resolver
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.hostEntries = []hostEntry{{hostname: "myhost.example.com", port: "53", transport: "dns"}}
	f.staticCount = 0

	// Initial resolution
	resolvedAddrs, err := resolveHostEntries(f.hostEntries, f.resolver)
	if err != nil {
		t.Fatalf("initial resolve error: %v", err)
	}
	for _, addr := range resolvedAddrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	f.OnStartup()
	defer f.OnShutdown()

	// Get initial proxy pointer
	f.proxyMu.RLock()
	initialProxy := f.proxies[0]
	f.proxyMu.RUnlock()

	// Trigger re-resolution with same result
	f.reResolve()

	// Proxy should not have changed (same IP returned)
	f.proxyMu.RLock()
	currentProxy := f.proxies[0]
	f.proxyMu.RUnlock()

	if initialProxy != currentProxy {
		t.Error("proxy should not have changed when IPs are the same")
	}
}

func TestSetupResolverWithProxyOptions(t *testing.T) {
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.1"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Build Forward manually with test server resolver and options
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.hostEntries = []hostEntry{{hostname: "myhost.example.com", port: "53", transport: "dns"}}
	f.maxfails = 3
	f.opts.ForceTCP = true
	f.opts.HCDomain = "example.org."
	f.opts.HCRecursionDesired = true

	// Resolve and create proxies
	resolvedAddrs, err := resolveHostEntries(f.hostEntries, f.resolver)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	for _, addr := range resolvedAddrs {
		host, zone := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.configureProxy(p, trans, zone)
		f.proxies = append(f.proxies, p)
	}

	if f.maxfails != 3 {
		t.Errorf("expected maxfails 3, got %d", f.maxfails)
	}
	if !f.opts.ForceTCP {
		t.Error("expected ForceTCP to be true")
	}
	if f.opts.HCDomain != "example.org." {
		t.Errorf("expected HCDomain example.org., got %s", f.opts.HCDomain)
	}

	// Verify proxy options are applied via configureProxy
	p := f.proxies[0]
	if p.GetHealthchecker().GetDomain() != "example.org." {
		t.Errorf("expected healthcheck domain example.org., got %s", p.GetHealthchecker().GetDomain())
	}
	if !p.GetHealthchecker().GetRecursionDesired() {
		t.Error("expected recursion desired to be true")
	}
}

func TestResolverWithHCOptions(t *testing.T) {
	input := "forward . 127.0.0.1 {\nresolver 10.96.0.10\n}\n"

	c := caddy.NewTestController("dns", input)
	fs, err := parseForward(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f := fs[0]
	if len(f.resolver) != 1 || f.resolver[0] != "10.96.0.10" {
		t.Errorf("unexpected resolver: %v", f.resolver)
	}

	// Test the proxy opts are set (default values)
	expectedOpts := proxy.Options{HCRecursionDesired: true, HCDomain: "."}
	if f.opts != expectedOpts {
		t.Errorf("expected opts %v, got %v", expectedOpts, f.opts)
	}
}
