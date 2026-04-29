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
		name        string
		input       []string
		wantStatic  int
		wantDynamic int
		wantErr     bool
		errContains string
	}{
		{
			name:       "simple IP",
			input:      []string{"127.0.0.1"},
			wantStatic: 1,
		},
		{
			name:       "IP with port",
			input:      []string{"127.0.0.1:8053"},
			wantStatic: 1,
		},
		{
			name:       "IPv6",
			input:      []string{"::1"},
			wantStatic: 1,
		},
		{
			name:       "TLS IP",
			input:      []string{"tls://127.0.0.1"},
			wantStatic: 1,
		},
		{
			name:       "resolv.conf file",
			input:      []string{resolv},
			wantStatic: 1,
		},
		{
			name:        "hostname",
			input:       []string{"dns.example.com"},
			wantDynamic: 1,
		},
		{
			name:        "hostname with port",
			input:       []string{"dns.example.com:5353"},
			wantDynamic: 1,
		},
		{
			name:        "TLS hostname",
			input:       []string{"tls://dns.example.com"},
			wantDynamic: 1,
		},
		{
			name:        "k8s service name",
			input:       []string{"rbldnsd.rbldnsd.svc.cluster.local"},
			wantDynamic: 1,
		},
		{
			name:        "mixed IPs and hostnames",
			input:       []string{"127.0.0.1", "dns.example.com", "10.0.0.1"},
			wantStatic:  2,
			wantDynamic: 1,
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
			entries, err := classifyToAddrs(tc.input)
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

			staticCount := 0
			dynamicCount := 0
			for _, e := range entries {
				if e.static {
					staticCount++
				} else {
					dynamicCount++
				}
			}
			if staticCount != tc.wantStatic {
				t.Errorf("expected %d static entries, got %d", tc.wantStatic, staticCount)
			}
			if dynamicCount != tc.wantDynamic {
				t.Errorf("expected %d dynamic entries, got %d", tc.wantDynamic, dynamicCount)
			}
		})
	}
}

func TestClassifyToAddrsPreservesOrder(t *testing.T) {
	entries, err := classifyToAddrs([]string{"dns.example.com", "127.0.0.1", "other.example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].static || entries[0].entry.hostname != "dns.example.com" {
		t.Errorf("entry 0: expected dynamic dns.example.com, got static=%v entry=%v", entries[0].static, entries[0].entry)
	}
	if !entries[1].static || entries[1].addrs[0] != "127.0.0.1:53" {
		t.Errorf("entry 1: expected static 127.0.0.1:53, got static=%v addrs=%v", entries[1].static, entries[1].addrs)
	}
	if entries[2].static || entries[2].entry.hostname != "other.example.com" {
		t.Errorf("entry 2: expected dynamic other.example.com, got static=%v entry=%v", entries[2].static, entries[2].entry)
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

func TestExpandAndDedup(t *testing.T) {
	// Start a test DNS server that returns different IPs for different hostnames
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			switch r.Question[0].Name {
			case "host1.example.com.":
				ret.Answer = append(ret.Answer,
					test.A("host1.example.com. IN A 10.0.0.1"),
					test.A("host1.example.com. IN A 10.0.0.2"),
				)
			case "host2.example.com.":
				ret.Answer = append(ret.Answer,
					test.A("host2.example.com. IN A 10.0.0.2"),
					test.A("host2.example.com. IN A 10.0.0.3"),
				)
			}
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Simulate: forward . host1(→10.0.0.1,10.0.0.2) host2(→10.0.0.2,10.0.0.3) 10.0.0.3 10.0.0.2
	entries := []toEntry{
		{static: false, entry: hostEntry{hostname: "host1.example.com", port: "53", transport: "dns"}},
		{static: false, entry: hostEntry{hostname: "host2.example.com", port: "53", transport: "dns"}},
		{static: true, addrs: []string{"10.0.0.3:53"}},
		{static: true, addrs: []string{"10.0.0.2:53"}},
	}

	result, err := expandAndDedup(entries, []string{s.Addr}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected: 10.0.0.1, 10.0.0.2, 10.0.0.3 (first-seen order, deduped)
	expected := []string{"10.0.0.1:53", "10.0.0.2:53", "10.0.0.3:53"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d addresses, got %d: %v", len(expected), len(result), result)
	}
	for i, addr := range result {
		normalized := normalizeAddr(addr)
		if normalized != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], normalized)
		}
	}
}

func TestExpandAndDedupOrderPreserved(t *testing.T) {
	// Start a test DNS server
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			ret.Answer = append(ret.Answer, test.A("myhost.example.com. IN A 10.0.0.42"))
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	// Config order: hostname first, then static IP
	// forward . myhost.example.com 192.168.1.1
	entries := []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
		{static: true, addrs: []string{"192.168.1.1:53"}},
	}

	result, err := expandAndDedup(entries, []string{s.Addr}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// hostname resolved IP should come first, then static
	if len(result) != 2 {
		t.Fatalf("expected 2 addresses, got %d: %v", len(result), result)
	}
	if normalizeAddr(result[0]) != "10.0.0.42:53" {
		t.Errorf("expected first addr 10.0.0.42:53, got %s", normalizeAddr(result[0]))
	}
	if normalizeAddr(result[1]) != "192.168.1.1:53" {
		t.Errorf("expected second addr 192.168.1.1:53, got %s", normalizeAddr(result[1]))
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
	ips, _, err := dnsLookup("myhost.example.com", []string{s.Addr})
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
			name:     "default interval (disabled)",
			input:    "forward . 127.0.0.1\n",
			expected: 0 * time.Second,
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
			name:     "zero disables re-resolution",
			input:    "forward . 127.0.0.1 {\nresolve_interval 0s\n}\n",
			expected: 0 * time.Second,
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

	// Test resolving a hostname entry directly
	entry := hostEntry{hostname: "myupstream.example.com", port: "53", transport: "dns"}
	addrs, _, err := resolveHostEntry(entry, []string{s.Addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("expected at least one resolved address")
	}
	if addrs[0] != "10.0.0.42:53" {
		t.Errorf("expected resolved addr 10.0.0.42:53, got %s", addrs[0])
	}

	// Test full integration: manually build the Forward with resolver
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: entry},
	}

	resolvedAddrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("resolution failed: %v", err)
	}

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

	// Manually build Forward to test mixed hostname + IP (hostname first for order test)
	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myupstream.example.com", port: "53", transport: "dns"}},
		{static: true, addrs: []string{"127.0.0.1:53"}},
	}

	resolvedAddrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("expand error: %v", err)
	}

	for _, addr := range resolvedAddrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	// Should have 2 proxies: resolved hostname first, then static IP
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(f.proxies))
	}

	if f.proxies[0].Addr() != "10.0.0.42:53" {
		t.Errorf("expected first proxy 10.0.0.42:53, got %s", f.proxies[0].Addr())
	}
	if f.proxies[1].Addr() != "127.0.0.1:53" {
		t.Errorf("expected second proxy 127.0.0.1:53, got %s", f.proxies[1].Addr())
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
			var rr *dns.A
			if callCount <= 2 {
				rr = test.A("myhost.example.com. IN A 10.0.0.1")
			} else {
				rr = test.A("myhost.example.com. IN A 10.0.0.2")
			}
			rr.Hdr.Ttl = 0 // TTL=0 so re-resolve always happens
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: true, addrs: []string{"127.0.0.1:53"}},
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	// Initial expand
	addrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	f.OnStartup()
	defer f.OnShutdown()

	// Initial state: 2 proxies (static + resolved)
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(f.proxies))
	}
	// Order: static first, then hostname
	if f.proxies[0].Addr() != "127.0.0.1:53" {
		t.Errorf("expected first proxy 127.0.0.1:53, got %s", f.proxies[0].Addr())
	}
	if f.proxies[1].Addr() != "10.0.0.1:53" {
		t.Errorf("expected second proxy 10.0.0.1:53, got %s", f.proxies[1].Addr())
	}

	// Trigger re-resolution
	f.reResolve()

	f.proxyMu.RLock()
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 proxies after re-resolve, got %d", len(f.proxies))
	}
	// Order preserved: static first, then new resolved
	firstAddr := f.proxies[0].Addr()
	secondAddr := f.proxies[1].Addr()
	f.proxyMu.RUnlock()

	if firstAddr != "127.0.0.1:53" {
		t.Errorf("expected first proxy still 127.0.0.1:53, got %s", firstAddr)
	}
	if secondAddr != "10.0.0.2:53" {
		t.Errorf("expected re-resolved proxy 10.0.0.2:53, got %s", secondAddr)
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

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	f.OnStartup()
	defer f.OnShutdown()

	f.proxyMu.RLock()
	initialProxy := f.proxies[0]
	f.proxyMu.RUnlock()

	// Trigger re-resolution with same result
	f.reResolve()

	f.proxyMu.RLock()
	currentProxy := f.proxies[0]
	f.proxyMu.RUnlock()

	if initialProxy != currentProxy {
		t.Error("proxy should not have changed when IPs are the same")
	}
}

func TestReResolveDedup(t *testing.T) {
	// Test that dedup works during re-resolve: if a hostname's IP changes
	// to one already present from another source, it gets deduped.
	callCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		callCount++
		if r.Question[0].Qtype == dns.TypeA {
			var rr *dns.A
			if callCount <= 2 {
				// Initial: hostname resolves to 10.0.0.99
				rr = test.A("myhost.example.com. IN A 10.0.0.99")
			} else {
				// After: hostname resolves to 10.0.0.1 (same as static)
				rr = test.A("myhost.example.com. IN A 10.0.0.1")
			}
			rr.Hdr.Ttl = 0 // TTL=0 so re-resolve always happens
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
		{static: true, addrs: []string{"10.0.0.1:53"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	f.OnStartup()
	defer f.OnShutdown()

	// Initial: 2 proxies (10.0.0.99 from hostname, 10.0.0.1 from static)
	if len(f.proxies) != 2 {
		t.Fatalf("expected 2 initial proxies, got %d", len(f.proxies))
	}

	// Trigger re-resolve: hostname now returns 10.0.0.1 (same as static)
	f.reResolve()

	f.proxyMu.RLock()
	proxyCount := len(f.proxies)
	f.proxyMu.RUnlock()

	// After dedup: should be 1 proxy (10.0.0.1 seen from hostname first, static deduped)
	if proxyCount != 1 {
		t.Errorf("expected 1 proxy after dedup, got %d", proxyCount)
	}
}

func TestReResolveNoChangeOnReorder(t *testing.T) {
	// DNS returns IPs in different order on re-resolve, but same set.
	// This should NOT trigger a proxy rebuild.
	callCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		callCount++
		if r.Question[0].Qtype == dns.TypeA {
			if callCount <= 2 {
				ret.Answer = append(ret.Answer,
					test.A("myhost.example.com. IN A 10.0.0.1"),
					test.A("myhost.example.com. IN A 10.0.0.2"),
				)
			} else {
				// Same IPs, different order
				ret.Answer = append(ret.Answer,
					test.A("myhost.example.com. IN A 10.0.0.2"),
					test.A("myhost.example.com. IN A 10.0.0.1"),
				)
			}
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	f.OnStartup()
	defer f.OnShutdown()

	// Save initial proxy pointers
	f.proxyMu.RLock()
	initialProxies := make([]*proxy.Proxy, len(f.proxies))
	copy(initialProxies, f.proxies)
	f.proxyMu.RUnlock()

	// Re-resolve — same IPs but different order from DNS
	f.reResolve()

	// Proxies should NOT have been replaced
	f.proxyMu.RLock()
	for i, p := range f.proxies {
		if p != initialProxies[i] {
			t.Errorf("proxy %d was replaced despite same IP set", i)
		}
	}
	f.proxyMu.RUnlock()
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

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}
	f.maxfails = 3
	f.opts.ForceTCP = true
	f.opts.HCDomain = "example.org."
	f.opts.HCRecursionDesired = true

	addrs, err := expandAndDedup(f.toEntries, f.resolver, 0)
	if err != nil {
		t.Fatalf("expand error: %v", err)
	}
	for _, addr := range addrs {
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

	p := f.proxies[0]
	if p.GetHealthchecker().GetDomain() != "example.org." {
		t.Errorf("expected healthcheck domain example.org., got %s", p.GetHealthchecker().GetDomain())
	}
	if !p.GetHealthchecker().GetRecursionDesired() {
		t.Error("expected recursion desired to be true")
	}
}

func TestExpandAndDedupTLS(t *testing.T) {
	// tls://hostname1(A 9.9.9.9, A 149.112.112.112) hostname2(A 149.112.112.112, A 9.9.9.10) 149.112.112.112 9.9.9.10
	// Expected after dedup: 9.9.9.9 149.112.112.112 9.9.9.10 (first-seen order)
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			switch r.Question[0].Name {
			case "dns1.example.com.":
				ret.Answer = append(ret.Answer,
					test.A("dns1.example.com. IN A 9.9.9.9"),
					test.A("dns1.example.com. IN A 149.112.112.112"),
				)
			case "dns2.example.com.":
				ret.Answer = append(ret.Answer,
					test.A("dns2.example.com. IN A 149.112.112.112"),
					test.A("dns2.example.com. IN A 9.9.9.10"),
				)
			}
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	entries := []toEntry{
		{static: false, entry: hostEntry{hostname: "dns1.example.com", port: "853", transport: "tls"}},
		{static: false, entry: hostEntry{hostname: "dns2.example.com", port: "853", transport: "tls"}},
		{static: true, addrs: []string{"tls://149.112.112.112:853"}},
		{static: true, addrs: []string{"tls://9.9.9.10:853"}},
	}

	result, err := expandAndDedup(entries, []string{s.Addr}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"9.9.9.9:853", "149.112.112.112:853", "9.9.9.10:853"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d addresses after dedup, got %d: %v", len(expected), len(result), result)
	}
	for i, addr := range result {
		if normalizeAddr(addr) != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], normalizeAddr(addr))
		}
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

	expectedOpts := proxy.Options{HCRecursionDesired: true, HCDomain: "."}
	if f.opts != expectedOpts {
		t.Errorf("expected opts %v, got %v", expectedOpts, f.opts)
	}
}

func TestReResolveTTLHonored(t *testing.T) {
	// DNS server returns records with TTL=3600. On re-resolve, entries with
	// valid TTL should be skipped (cached addresses used).
	resolveCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		resolveCount++
		if r.Question[0].Qtype == dns.TypeA {
			rr := test.A("myhost.example.com. IN A 10.0.0.1")
			rr.Hdr.Ttl = 3600
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.resolveInterval = 5 * time.Second
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	// Initial expand sets TTL
	addrs, err := expandAndDedup(f.toEntries, f.resolver, f.resolveHold)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	initialResolveCount := resolveCount

	// Trigger re-resolve — TTL is 3600s, should skip and use cached
	f.reResolve()

	if resolveCount != initialResolveCount {
		t.Errorf("expected no new DNS queries (TTL valid), but resolve count went from %d to %d",
			initialResolveCount, resolveCount)
	}
}

func TestReResolveTTLExpired(t *testing.T) {
	// DNS server returns TTL=1. After expiry, re-resolve should query DNS again.
	resolveCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		resolveCount++
		if r.Question[0].Qtype == dns.TypeA {
			rr := test.A("myhost.example.com. IN A 10.0.0.1")
			rr.Hdr.Ttl = 1
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.resolveInterval = 5 * time.Second
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, f.resolveHold)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	initialResolveCount := resolveCount

	// Wait for TTL to expire
	time.Sleep(1100 * time.Millisecond)

	// Trigger re-resolve — TTL expired, should query DNS
	f.reResolve()

	if resolveCount == initialResolveCount {
		t.Error("expected new DNS queries after TTL expired, but none happened")
	}
}

func TestResolveHoldCapsTTL(t *testing.T) {
	// DNS returns TTL=3600 but resolve_hold=1s caps it.
	// After hold expires, re-resolve should query DNS again.
	resolveCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		resolveCount++
		if r.Question[0].Qtype == dns.TypeA {
			rr := test.A("myhost.example.com. IN A 10.0.0.1")
			rr.Hdr.Ttl = 3600
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.resolveInterval = 5 * time.Second
	f.resolveHold = 1 * time.Second // cap TTL to 1s
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "myhost.example.com", port: "53", transport: "dns"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, f.resolveHold)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	initialResolveCount := resolveCount

	// TTL=3600 but hold=1s — immediately should still be cached
	f.reResolve()
	if resolveCount != initialResolveCount {
		t.Error("expected cached result immediately after expand")
	}

	// Wait for hold to expire
	time.Sleep(1100 * time.Millisecond)

	f.reResolve()
	if resolveCount == initialResolveCount {
		t.Error("expected new DNS queries after resolve_hold expired, but none happened")
	}
}

func TestReResolveTTLDedupWithMixedEntries(t *testing.T) {
	// Synthetic test: two dynamic entries, one with TTL expired and one still valid.
	// The expired one changes IP to match a static entry — dedup must still work.
	callCount := 0
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		callCount++
		if r.Question[0].Qtype == dns.TypeA {
			switch r.Question[0].Name {
			case "host1.example.com.":
				rr := test.A("host1.example.com. IN A 10.0.0.1")
				rr.Hdr.Ttl = 3600 // long TTL — won't re-resolve
				ret.Answer = append(ret.Answer, rr)
			case "host2.example.com.":
				rr := test.A("host2.example.com. IN A 10.0.0.99")
				rr.Hdr.Ttl = 1 // short TTL — will expire
				if callCount > 4 {
					// After expiry, host2 changes to 10.0.0.1 (same as host1 = dedup)
					rr = test.A("host2.example.com. IN A 10.0.0.1")
					rr.Hdr.Ttl = 1
				}
				ret.Answer = append(ret.Answer, rr)
			}
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	f := New()
	f.from = "."
	f.resolver = []string{s.Addr}
	f.resolveInterval = 5 * time.Second
	f.toEntries = []toEntry{
		{static: false, entry: hostEntry{hostname: "host1.example.com", port: "53", transport: "dns"}},
		{static: false, entry: hostEntry{hostname: "host2.example.com", port: "53", transport: "dns"}},
		{static: true, addrs: []string{"10.0.0.50:53"}},
	}

	addrs, err := expandAndDedup(f.toEntries, f.resolver, f.resolveHold)
	if err != nil {
		t.Fatalf("initial expand error: %v", err)
	}
	for _, addr := range addrs {
		host, _ := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.proxies = append(f.proxies, p)
	}

	// Initial: host1=10.0.0.1, host2=10.0.0.99, static=10.0.0.50 → 3 proxies
	f.proxyMu.RLock()
	if len(f.proxies) != 3 {
		t.Fatalf("expected 3 initial proxies, got %d", len(f.proxies))
	}
	f.proxyMu.RUnlock()

	// Wait for host2 TTL to expire (host1 TTL=3600 stays valid)
	time.Sleep(1100 * time.Millisecond)

	// Re-resolve: host1 uses cache (10.0.0.1), host2 re-resolves to 10.0.0.1 (deduped)
	f.reResolve()

	f.proxyMu.RLock()
	proxyCount := len(f.proxies)
	var proxyAddrs []string
	for _, p := range f.proxies {
		proxyAddrs = append(proxyAddrs, p.Addr())
	}
	f.proxyMu.RUnlock()

	// After dedup: host1=10.0.0.1, host2=10.0.0.1 (deduped), static=10.0.0.50 → 2 proxies
	if proxyCount != 2 {
		t.Errorf("expected 2 proxies after dedup, got %d: %v", proxyCount, proxyAddrs)
	}
}

func TestSetupResolveHold(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldErr   bool
		expectedErr string
		expected    time.Duration
	}{
		{
			name:     "default hold (disabled)",
			input:    "forward . 127.0.0.1\n",
			expected: 0 * time.Second,
		},
		{
			name:     "custom hold",
			input:    "forward . 127.0.0.1 {\nresolve_hold 60s\n}\n",
			expected: 60 * time.Second,
		},
		{
			name:     "hold 5 minutes",
			input:    "forward . 127.0.0.1 {\nresolve_hold 5m\n}\n",
			expected: 5 * time.Minute,
		},
		{
			name:     "hold zero (no cap)",
			input:    "forward . 127.0.0.1 {\nresolve_hold 0s\n}\n",
			expected: 0 * time.Second,
		},
		{
			name:        "negative hold",
			input:       "forward . 127.0.0.1 {\nresolve_hold -5s\n}\n",
			shouldErr:   true,
			expectedErr: "can't be negative",
		},
		{
			name:        "invalid duration",
			input:       "forward . 127.0.0.1 {\nresolve_hold notaduration\n}\n",
			shouldErr:   true,
			expectedErr: "invalid duration",
		},
		{
			name:        "no args",
			input:       "forward . 127.0.0.1 {\nresolve_hold\n}\n",
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
			if f.resolveHold != tc.expected {
				t.Errorf("expected resolve_hold %v, got %v", tc.expected, f.resolveHold)
			}
		})
	}
}

func TestDnsLookupReturnsTTL(t *testing.T) {
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			rr := test.A("myhost.example.com. IN A 10.0.0.42")
			rr.Hdr.Ttl = 300
			ret.Answer = append(ret.Answer, rr)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	ips, ttl, err := dnsLookup("myhost.example.com", []string{s.Addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("expected at least one IP")
	}
	if ttl != 300 {
		t.Errorf("expected TTL 300, got %d", ttl)
	}
}

func TestDnsLookupReturnsMinTTL(t *testing.T) {
	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			rr1 := test.A("myhost.example.com. IN A 10.0.0.1")
			rr1.Hdr.Ttl = 600
			rr2 := test.A("myhost.example.com. IN A 10.0.0.2")
			rr2.Hdr.Ttl = 120
			ret.Answer = append(ret.Answer, rr1, rr2)
		}
		w.WriteMsg(ret)
	})
	defer s.Close()

	_, ttl, err := dnsLookup("myhost.example.com", []string{s.Addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl != 120 {
		t.Errorf("expected min TTL 120, got %d", ttl)
	}
}

func TestTtlToExpiry(t *testing.T) {
	now := time.Now()

	// TTL 0 → zero time (no TTL info)
	exp := ttlToExpiry(0, 0)
	if !exp.IsZero() {
		t.Errorf("expected zero time for TTL=0, got %v", exp)
	}

	// TTL 300, no hold → ~300s from now
	exp = ttlToExpiry(300, 0)
	diff := exp.Sub(now)
	if diff < 299*time.Second || diff > 301*time.Second {
		t.Errorf("expected ~300s expiry, got %v", diff)
	}

	// TTL 3600, hold 60s → ~60s from now (capped)
	exp = ttlToExpiry(3600, 60*time.Second)
	diff = exp.Sub(now)
	if diff < 59*time.Second || diff > 61*time.Second {
		t.Errorf("expected ~60s expiry (capped), got %v", diff)
	}

	// TTL 30, hold 60s → ~30s from now (TTL < hold, no cap needed)
	exp = ttlToExpiry(30, 60*time.Second)
	diff = exp.Sub(now)
	if diff < 29*time.Second || diff > 31*time.Second {
		t.Errorf("expected ~30s expiry (under hold), got %v", diff)
	}
}
