package forward

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	test "github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

func TestIsValidPort(t *testing.T) {
	testCases := []struct {
		port     string
		expected bool
	}{
		{"1", true},
		{"80", true},
		{"443", true},
		{"8080", true},
		{"65535", true},
		{"0", false},     // Invalid - port 0
		{"65536", false}, // Invalid - too high
		{"-1", false},    // Invalid - negative
		{"", false},      // Invalid - empty
		{" ", false},     // Invalid - whitespace
		{"abc", false},   // Invalid - not numeric
		{"8080a", false}, // Invalid - not purely numeric
	}

	for _, tc := range testCases {
		t.Run(tc.port, func(t *testing.T) {
			result := isValidPort(tc.port)
			if result != tc.expected {
				t.Errorf("isValidPort(%q) = %v, want %v", tc.port, result, tc.expected)
			}
		})
	}
}

func TestParseGatewayURL(t *testing.T) {
	testCases := []struct {
		input     string
		wantTrans string
		wantHost  string
		wantPort  string
		wantErr   bool
	}{
		// Standard formats
		{"do53://example.com:53", "dns", "example.com.", "53", false},
		{"doT://example.com:853", "tls", "example.com.", "853", false},

		// Default ports
		{"Do53://example.com", "dns", "example.com.", "53", false},
		{"DoT://example.com", "tls", "example.com.", "853", false},

		// IP addresses
		{"Do53://1.1.1.1:53", "dns", "1.1.1.1", "53", false},
		{"DoT://9.9.9.9", "tls", "9.9.9.9", "853", false},

		// Error cases
		{"", "", "", "", true},                   // Empty string
		{"example", "", "", "", true},            // Missing scheme and port
		{"://example.com:853", "", "", "", true}, // Missing scheme
		{"example.com:53", "", "", "", true},
		{"invalid://example.com:53", "", "", "", true},                      // Invalid scheme
		{"Do53://:53", "", "", "", true},                                    // Missing host
		{"Do53://example.com", "dns", "example.com.", "53", false},          // Invalid port
		{"Do53://example.com:port", "", "", "", true},                       // port of type string
		{"invalid://example.com:65536", "", "", "", true},                   // Port out of range
		{"Do53://examplecom:53", "", "", "", true},                          // no trailing dot to a hostname which doesn't contain a single dot
		{"dns:53", "", "", "", true},                                        // Invalid format, missing scheme
		{"Do53://abc$edgh.com:53", "", "", "", true},                        // Invalid characters in hostname
		{"Do53://-abcgh.com:53", "", "", "", true},                          // Invalid hostname starting with hyphen
		{"Do53://1abc.com:53", "dns", "1abc.com.", "53", false},             // warn when fqdn starts with a digit
		{"Do53://example123.com:53", "dns", "example123.com.", "53", false}, // Valid DNS URL
		{"Do53://abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmno.example.com:53", "", "", "", true}, // Hostname too long
		{"Do53://example.123:53", "", "", "", true},  // Invalid hostname with digits
		{"Do53://example..com:53", "", "", "", true}, // Invalid hostname with consecutive dots (empty label)
		{"Do53://.example.com:53", "", "", "", true}, // Leading dot in domain (invalid)
		{"Do53://" + strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("c", 64) + "." + strings.Repeat("d", 64) + ".com:53", "", "", "", true}, // Domain name exceeds 255 characters
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			trans, host, port, err := parseGatewayURL(tc.input)

			if tc.wantErr && err == nil {
				t.Errorf("parseGatewayURL(%q) expected error, got nil", tc.input)
				return
			}

			if !tc.wantErr && err != nil {
				t.Errorf("parseGatewayURL(%q) unexpected error: %v", tc.input, err)
				return
			}

			if err == nil {
				if trans != tc.wantTrans {
					t.Errorf("parseGatewayURL(%q) got transport %q, want %q",
						tc.input, trans, tc.wantTrans)
				}

				if host != tc.wantHost {
					t.Errorf("parseGatewayURL(%q) got host %q, want %q",
						tc.input, host, tc.wantHost)
				}

				if port != tc.wantPort {
					t.Errorf("parseGatewayURL(%q) got port %q, want %q",
						tc.input, port, tc.wantPort)
				}
			}
		})
	}
}

func TestGetCustomProxies(t *testing.T) {
	// Create a Forward plugin
	f := New()

	// Create a test DNS server for resolution
	backend := dnstest.NewServer(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)

		// Handle A record queries for dns.google.
		if req.Question[0].Name == "dns.google." && req.Question[0].Qtype == dns.TypeA {
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "dns.google.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("8.8.8.8"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "dns.google.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("8.8.4.4"),
				},
			}
		}

		// Handle A record queries for dns.quad9.net.
		if req.Question[0].Name == "dns.quad9.net." && req.Question[0].Qtype == dns.TypeA {
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "dns.quad9.net.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("9.9.9.9"),
				},
			}
		}

		w.WriteMsg(resp)
	})
	defer backend.Close()

	// Set up the forward plugin to use our test server for resolution
	defaultProxy := proxy.NewProxy("default", backend.Addr, "dns")
	f.SetProxy(defaultProxy)

	testCases := []struct {
		name            string
		gateways        []string
		expectedProxies int
		appendDefaults  bool
	}{
		{
			name:            "No gateways",
			gateways:        nil,
			expectedProxies: 0,
			appendDefaults:  true,
		},
		{
			name:            "Single DNS gateway",
			gateways:        []string{"Do53://dns.google:53"},
			expectedProxies: 2, // 8.8.8.8:53 and 8.8.4.4:53
			appendDefaults:  false,
		},
		{
			name:            "TLS gateway",
			gateways:        []string{"DoT://dns.quad9.net:853"},
			expectedProxies: 1, // 9.9.9.9:853
			appendDefaults:  false,
		},
		{
			name:            "Direct IP gateway",
			gateways:        []string{"Do53://1.1.1.1:53"},
			expectedProxies: 1, // 1.1.1.1:53
			appendDefaults:  false,
		},
		{
			name:            "Infoblox indicator",
			gateways:        []string{"Do53://infoblox.com:53"},
			expectedProxies: 0,
			appendDefaults:  true,
		},
		{
			name:            "Mixed gateways with infoblox",
			gateways:        []string{"Do53://1.1.1.1:53", "Do53://infoblox.com:53", "DoT://dns.quad9.net:853"},
			expectedProxies: 2, // 1.1.1.1:53 and 9.9.9.9:853
			appendDefaults:  true,
		},
		{
			name:            "empty gateways",
			gateways:        []string{},
			expectedProxies: 0,
			appendDefaults:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a context with custom resolvers
			ctx := context.Background()
			if tc.gateways != nil {
				ctx = context.WithValue(ctx, CustomResolverKey, tc.gateways)
			}

			// Clear the cache between tests
			proxyCache.cache = make(map[string]cachedProxy)

			// Call getCustomProxies
			proxies := f.getCustomProxies(ctx, &test.ResponseWriter{})

			// Verify the results
			if len(proxies) != tc.expectedProxies {
				t.Errorf("Expected %d proxies, got %d", tc.expectedProxies, len(proxies))
			}

			// Verify proxy addresses for specific test cases
			if tc.name == "Single DNS gateway" && len(proxies) == 2 {
				addresses := make(map[string]bool)
				for _, p := range proxies {
					addresses[p.Addr()] = true
				}

				if !addresses["8.8.8.8:53"] || !addresses["8.8.4.4:53"] {
					t.Errorf("Expected proxies for 8.8.8.8:53 and 8.8.4.4:53, got: %v", addresses)
				}
			}

			if tc.name == "TLS gateway" && len(proxies) == 1 {
				if proxies[0].Addr() != "9.9.9.9:853" {
					t.Errorf("Expected proxy for 9.9.9.9:853, got %s", proxies[0].Addr())
				}
			}

			if tc.name == "Direct IP gateway" && len(proxies) == 1 {
				if proxies[0].Addr() != "1.1.1.1:53" {
					t.Errorf("Expected proxy for 1.1.1.1:53, got %s", proxies[0].Addr())
				}
			}

			if tc.name == "Mixed gateways with infoblox" && len(proxies) == 2 {
				addresses := make(map[string]bool)
				for _, p := range proxies {
					addresses[p.Addr()] = true
				}

				if !addresses["1.1.1.1:53"] || !addresses["9.9.9.9:853"] {
					t.Errorf("Expected proxies for 1.1.1.1:53 and 9.9.9.9:853, got: %v", addresses)
				}
			}

			if tc.name == "Infoblox indicator" {
				// No proxies expected for infoblox indicator
				if len(proxies) != 0 {
					t.Errorf("Expected 0 proxies for Infoblox indicator, got %d", len(proxies))
				}
				return
			}

			if tc.name == "empty gateways" && len(proxies) == 0 {
				// No proxies expected for empty gateways
				if len(proxies) != 0 {
					t.Errorf("Expected 0 proxies for empty gateways, got %d", len(proxies))
				}
				return
			}
		})
	}
}

func TestExtractIPsAndTTL(t *testing.T) {
	testCases := []struct {
		name           string
		records        []dns.RR
		port           string
		expectedIPs    map[string]struct{}
		expectedMinTTL time.Duration
	}{
		{
			name: "Single A record",
			records: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("192.0.2.1"),
				},
			},
			port: "53",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:53": {},
			},
			expectedMinTTL: 300 * time.Second,
		},
		{
			name: "Multiple A records",
			records: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("192.0.2.1"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    600,
					},
					A: net.ParseIP("192.0.2.2"),
				},
			},
			port: "53",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:53": {},
				"192.0.2.2:53": {},
			},
			expectedMinTTL: 300 * time.Second, // Min of 300 and 600
		},
		{
			name: "A and AAAA records",
			records: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("192.0.2.1"),
				},
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    400,
					},
					AAAA: net.ParseIP("2001:db8::1"),
				},
			},
			port: "53",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:53":     {},
				"[2001:db8::1]:53": {},
			},
			expectedMinTTL: 300 * time.Second, // Min of 300 and 400
		},
		{
			name:           "Empty records",
			records:        []dns.RR{},
			port:           "53",
			expectedIPs:    map[string]struct{}{},
			expectedMinTTL: 365 * 24 * time.Hour,
		},
		{
			name: "Non-IP records",
			records: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Target: "target.example.com.",
				},
			},
			port:           "53",
			expectedIPs:    map[string]struct{}{},
			expectedMinTTL: 365 * 24 * time.Hour,
		},
		{
			name: "Custom port",
			records: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("192.0.2.1"),
				},
			},
			port: "8053",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:8053": {},
			},
			expectedMinTTL: 300 * time.Second,
		},
	}

	for _, tc := range testCases {
		var addresses = make(map[string]struct{})
		t.Run(tc.name, func(t *testing.T) {
			ttl := extractIPsAndTTL(tc.records, tc.port, addresses)

			// Compare addresses
			if len(addresses) != len(tc.expectedIPs) {
				t.Errorf("Expected %d addresses, got %d", len(tc.expectedIPs), len(addresses))
			}

			for addr := range tc.expectedIPs {
				if _, exists := addresses[addr]; !exists {
					t.Errorf("Expected address %s not found in result", addr)
				}
			}

			// Compare TTL
			if ttl != tc.expectedMinTTL {
				t.Errorf("Expected min TTL %v, got %v", tc.expectedMinTTL, ttl)
			}
		})
	}
}

func TestBuildProxy(t *testing.T) {
	f := New()

	testCases := []struct {
		name          string
		domainName    string
		ipAddress     string
		transport     string
		shouldBeValid bool
	}{
		{
			name:          "Valid DNS proxy",
			domainName:    "dns.example.com.",
			ipAddress:     "192.0.2.1:53",
			transport:     "dns",
			shouldBeValid: true,
		},
		{
			name:          "Valid TLS proxy",
			domainName:    "tls.example.com.",
			ipAddress:     "192.0.2.2:853",
			transport:     "tls",
			shouldBeValid: true,
		},
		{
			name:          "Valid IP address",
			domainName:    "192.0.2.1",
			ipAddress:     "192.0.2.1:853",
			transport:     "tls",
			shouldBeValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := f.buildProxy(tc.domainName, tc.ipAddress, tc.transport)

			if tc.shouldBeValid && proxy == nil {
				t.Error("Expected valid proxy, got nil")
			}

			if !tc.shouldBeValid && proxy != nil {
				t.Error("Expected nil proxy for invalid input, got non-nil")
			}

			if proxy != nil {
				if proxy.Addr() != tc.ipAddress {
					t.Errorf("Expected address %s, got %s", tc.ipAddress, proxy.Addr())
				}
			}
		})
	}
}

func TestCustomProxyCache(t *testing.T) {
	cache := &customProxyCache{
		cache: make(map[string]cachedProxy),
	}

	// Create some test proxies
	proxy1 := proxy.NewProxy("test1", "192.0.2.1:53", "dns")
	proxy2 := proxy.NewProxy("test2", "192.0.2.2:53", "dns")
	proxies := []*proxy.Proxy{proxy1, proxy2}

	// Test set/get with valid TTL
	gateway := "dns://example.com:53"
	futureExpiry := time.Now().Add(10 * time.Minute)
	cache.set(gateway, proxies, futureExpiry)

	gotProxies, valid := cache.get(gateway)
	if !valid {
		t.Error("Expected cache entry to be valid, but it was not")
	}

	if len(gotProxies) != 2 {
		t.Errorf("Expected 2 proxies, got %d", len(gotProxies))
	}

	// Test expired entry
	expiredGateway := "dns://expired.com:53"
	pastExpiry := time.Now().Add(-1 * time.Minute)
	cache.set(expiredGateway, proxies, pastExpiry)

	_, valid = cache.get(expiredGateway)
	if valid {
		t.Error("Expected cache entry to be invalid due to expiration, but it was valid")
	}

	// Test zero expiry time (should not be cached)
	zeroExpiryGateway := "dns://zero.com:53"
	zeroExpiry := time.Now() // Zero time
	cache.set(zeroExpiryGateway, proxies, zeroExpiry)

	// Verify entry wasn't cached
	_, valid = cache.get(zeroExpiryGateway)
	if valid {
		t.Error("Expected zero expiry entry to not be cached, but it was found in cache")
	}

	// Test removeEntry
	cache.removeEntry(gateway)
	_, valid = cache.get(gateway)
	if valid {
		t.Error("Expected entry to be removed, but it was still present and valid")
	}
}

func TestFetchProxies(t *testing.T) {
	// Create a Forward plugin
	f := New()
	// Create a test DNS server for resolution
	backend := dnstest.NewServer(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)

		// We only need minimal test cases since specific resolution behavior
		// is tested in other functions
		if req.Question[0].Name == "example.com." && req.Question[0].Qtype == dns.TypeA {
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("1.2.3.4"),
				},
			}
		}
		if req.Question[0].Name == "changing.example.com." && req.Question[0].Qtype == dns.TypeA {
			// Return a different IP on subsequent calls to simulate an IP change
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "changing.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("192.0.2.200"), // New IP address
				},
			}
		}

		w.WriteMsg(resp)
	})
	defer backend.Close()

	// Set up the forward plugin to use our test server for resolution
	defaultProxy := proxy.NewProxy("default", backend.Addr, "dns")
	f.SetProxy(defaultProxy)

	testCases := []struct {
		name               string
		gateways           []string
		expectedProxies    int
		appendDefaults     bool
		setupCache         bool // If true, set up cache before the test
		testCachePurge     bool // If true, test cache purging on resolution failure
		testIPChange       bool // If true, test IP change behavior
		verifyNoCacheEntry bool // If true, verify no cache entry exists afterward
	}{
		{
			name:            "Empty gateway list",
			gateways:        []string{},
			expectedProxies: 0,
			appendDefaults:  false,
		},
		{
			name:            "Only infoblox",
			gateways:        []string{"Do53://infoblox.com:53"},
			expectedProxies: 0,
			appendDefaults:  true,
		},
		{
			name:            "Mix with infoblox",
			gateways:        []string{"Do53://example.com:53", "Do53://infoblox.com:53"},
			expectedProxies: 1,
			appendDefaults:  true,
		},
		{
			name:            "Invalid gateway URL",
			gateways:        []string{"://invalid"},
			expectedProxies: 0,
			appendDefaults:  false,
		},
		{
			name:            "Cache hit test",
			gateways:        []string{"Do53://cached.example.com:53"},
			expectedProxies: 1,
			appendDefaults:  false,
			setupCache:      true,
		},
		{
			name:            "DNS resolution failure existing expired cache entry",
			gateways:        []string{"Do53://nonexistent.example.com:53"},
			expectedProxies: 0,
			appendDefaults:  false,
			setupCache:      true, // Set up cache to test purging
			testCachePurge:  true, // flag to test cache purging
		},
		{
			name:               "DNS resolution failure non existing cache entry",
			gateways:           []string{"Do53://nonexistent.example.com:53"},
			expectedProxies:    0,
			appendDefaults:     false,
			setupCache:         false, // Set up cache to test purging
			verifyNoCacheEntry: true,  // For verifying no entry exists afterward
		},
		{
			name:            "IP change test",
			gateways:        []string{"Do53://changing.example.com:53"},
			expectedProxies: 1, // Should return the new IP address
			appendDefaults:  false,
			setupCache:      true, // Set up cache to test IP change
			testIPChange:    true, // Flag to test IP change behavior
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear the cache before each test
			proxyCache.cache = make(map[string]cachedProxy)

			// Set up cache if needed for this test case
			// For testing IP changes, create a proxy with the old IP first
			if tc.setupCache && (tc.testIPChange || tc.testCachePurge) {
				// Create a cached proxy with the original IP
				oldAddr := "192.0.2.100:53" // address before the change
				cachedProxy := proxy.NewProxy("cached", oldAddr, "dns")
				proxyCache.set(tc.gateways[0],
					[]*proxy.Proxy{cachedProxy},
					time.Now().Add(1*time.Second)) // Set to current time to simulate existing cache entry

			}
			if tc.setupCache && !tc.testIPChange && !tc.testCachePurge {
				cachedProxy := proxy.NewProxy("cached", "10.0.0.1:53", "dns")
				proxyCache.set(tc.gateways[0],
					[]*proxy.Proxy{cachedProxy},
					time.Now().Add(5*time.Minute))
			}

			time.Sleep(1100 * time.Millisecond) // Wait slightly longer than 1 second for cache expiration

			// Call fetchProxies directly
			proxies := f.fetchProxies(context.Background(), tc.gateways, &test.ResponseWriter{})

			// Verify the results
			if len(proxies) != tc.expectedProxies {
				t.Errorf("Expected %d proxies, got %d", tc.expectedProxies, len(proxies))
			}

			if tc.testIPChange {
				// Verify the new proxy has the correct IP
				expectedNewIP := "192.0.2.200:53"
				if proxies[0].Addr() != expectedNewIP {
					t.Errorf("Expected new proxy with IP %s, got %s",
						expectedNewIP, proxies[0].Addr())
				}
			}

			// Check specific case - cache purge on resolution failure
			if tc.testCachePurge {
				// The cache entry should be removed when DNS resolution fails
				cacheKey := tc.gateways[0]
				_, valid := proxyCache.get(cacheKey)
				if valid {
					t.Errorf("Expected cache entry to be removed after DNS resolution failure, but it's still present")
				}
			}

			// Check specific case - verify no cache entry exists after failed resolution
			if tc.verifyNoCacheEntry {
				cacheKey := tc.gateways[0]
				_, valid := proxyCache.get(cacheKey)
				if valid {
					t.Errorf("Expected no cache entry for failed resolution, but found one")
				}
			}

			// Check specific case - cache should be used
			if tc.name == "Cache hit test" && len(proxies) > 0 {
				if proxies[0].Addr() != "10.0.0.1:53" {
					t.Errorf("Cache wasn't used, got proxy with address %s", proxies[0].Addr())
				}
			}
		})
	}
}

func TestFetchProxies_CachingBehavior(t *testing.T) {
	f := New()

	// Set up a simple DNS server
	backend := dnstest.NewServer(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)

		if req.Question[0].Name == "example.com." && req.Question[0].Qtype == dns.TypeA {
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.ParseIP("5.5.5.5"),
				},
			}
		}

		w.WriteMsg(resp)
	})
	defer backend.Close()

	defaultProxy := proxy.NewProxy("default", backend.Addr, "dns")
	f.SetProxy(defaultProxy)

	// Clear the cache before testing
	proxyCache.cache = make(map[string]cachedProxy)

	// First fetch - should create new proxy and cache it
	gateway := []string{"Do53://example.com:53"}
	proxies1 := f.fetchProxies(context.Background(), gateway, &test.ResponseWriter{})

	// Cache should now contain an entry
	if len(proxyCache.cache) != 1 {
		t.Fatalf("Expected 1 cache entry, found %d", len(proxyCache.cache))
	}

	// Record the address of the first proxy
	firstProxyAddr := proxies1[0].Addr()

	// Second fetch - should use cache
	proxies2 := f.fetchProxies(context.Background(), gateway, &test.ResponseWriter{})

	// Should get the same proxy object (same address)
	if proxies2[0].Addr() != firstProxyAddr {
		t.Errorf("Expected cached proxy with address %s, got %s",
			firstProxyAddr, proxies2[0].Addr())
	}

	// Now force expiration of the cached entry
	for key := range proxyCache.cache {
		proxyCache.cache[key] = cachedProxy{
			proxies:    proxyCache.cache[key].proxies,
			expiryTime: time.Now().Add(-10 * time.Minute), // Expired
		}
	}

	// Third fetch - should create new proxy due to expiration
	proxies3 := f.fetchProxies(context.Background(), gateway, &test.ResponseWriter{})

	thirdProxyAddr := proxies3[0].Addr()
	log.Infof("Third proxy address: %s", thirdProxyAddr)
	// Should still have 1 entry in cache (replaced)
	if len(proxyCache.cache) != 1 {
		t.Fatalf("Expected 1 cache entry after refresh, found %d", len(proxyCache.cache))
	}

	// The entry should be fresh now
	for _, entry := range proxyCache.cache {
		if !entry.expiryTime.After(time.Now()) {
			t.Error("Entry should have been refreshed with future expiry time")
		}
	}
}

func TestResolveFqdnToIPs(t *testing.T) {
	// Create a Forward plugin
	f := New()

	// Create a test DNS server for resolution
	backend := dnstest.NewServer(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)

		// Handle A record queries
		if req.Question[0].Qtype == dns.TypeA {
			switch req.Question[0].Name {
			case "example.com.":
				resp.Answer = []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "example.com.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						A: net.ParseIP("192.0.2.1"),
					},
				}
			case "multi.example.com.":
				resp.Answer = []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "multi.example.com.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    600,
						},
						A: net.ParseIP("192.0.2.10"),
					},
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "multi.example.com.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    300, // Different TTL to test min TTL selection
						},
						A: net.ParseIP("192.0.2.11"),
					},
				}
			case "nonexistent.example.com.":
				// No records, empty answer
			}
		}

		// Handle AAAA record queries
		if req.Question[0].Qtype == dns.TypeAAAA {
			switch req.Question[0].Name {
			case "example.com.":
				resp.Answer = []dns.RR{
					&dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   "example.com.",
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    400,
						},
						AAAA: net.ParseIP("2001:db8::1"),
					},
				}
			case "ipv6only.example.com.":
				resp.Answer = []dns.RR{
					&dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   "ipv6only.example.com.",
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    500,
						},
						AAAA: net.ParseIP("2001:db8::2"),
					},
				}
			}
		}

		if req.Question[0].Name == "error.example.com." {
			// Don't respond at all - just sleep longer than the client's timeout
			time.Sleep(3 * time.Second) // Make sure this is longer than the client timeout
			return                      // Don't write any response
		}

		w.WriteMsg(resp)
	})
	defer backend.Close()

	// Set up the forward plugin to use our test server for resolution
	defaultProxy := proxy.NewProxy("default", backend.Addr, "dns")
	f.SetProxy(defaultProxy)

	testCases := []struct {
		name            string
		host            string
		port            string
		expectedIPs     map[string]struct{}
		expectedMinTTL  time.Duration
		expectedHasIPv4 bool
		expectedHasIPv6 bool
	}{
		{
			name: "Direct IP address",
			host: "1.2.3.4",
			port: "53",
			expectedIPs: map[string]struct{}{
				"1.2.3.4:53": {},
			},
			expectedMinTTL:  veryLongTTL,
			expectedHasIPv4: true,
			expectedHasIPv6: false,
		},
		{
			name: "Direct IPv6 address",
			host: "2001:db8::1",
			port: "853",
			expectedIPs: map[string]struct{}{
				"[2001:db8::1]:853": {},
			},
			expectedMinTTL:  veryLongTTL,
			expectedHasIPv4: false,
			expectedHasIPv6: true,
		},
		{
			name: "Hostname with both A and AAAA records",
			host: "example.com.",
			port: "53",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:53":     {},
				"[2001:db8::1]:53": {},
			},
			expectedMinTTL:  300 * time.Second, // Should take the minimum of 300 and 400
			expectedHasIPv4: true,
			expectedHasIPv6: true,
		},
		{
			name: "Hostname with multiple A records",
			host: "multi.example.com.",
			port: "53",
			expectedIPs: map[string]struct{}{
				"192.0.2.10:53": {},
				"192.0.2.11:53": {},
			},
			expectedMinTTL:  300 * time.Second, // Should take the minimum TTL
			expectedHasIPv4: true,
			expectedHasIPv6: false,
		},
		{
			name: "Hostname with only AAAA records",
			host: "ipv6only.example.com.",
			port: "53",
			expectedIPs: map[string]struct{}{
				"[2001:db8::2]:53": {},
			},
			expectedMinTTL:  500 * time.Second,
			expectedHasIPv4: false,
			expectedHasIPv6: true,
		},
		{
			name:            "Non-existent hostname",
			host:            "nonexistent.example.com.",
			port:            "53",
			expectedIPs:     map[string]struct{}{},
			expectedMinTTL:  0,
			expectedHasIPv4: false,
			expectedHasIPv6: false,
		},
		{
			name: "Custom port",
			host: "example.com.",
			port: "8053",
			expectedIPs: map[string]struct{}{
				"192.0.2.1:8053":     {},
				"[2001:db8::1]:8053": {},
			},
			expectedMinTTL:  300 * time.Second,
			expectedHasIPv4: true,
			expectedHasIPv6: true,
		},
		{
			name:            "DNS server error",
			host:            "error.example.com.",
			port:            "53",
			expectedIPs:     map[string]struct{}{},
			expectedMinTTL:  0,
			expectedHasIPv4: false,
			expectedHasIPv6: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			addresses, ttl := f.resolveFqdnToIPs(context.Background(), tc.host, tc.port, &test.ResponseWriter{})

			// Check if we got the expected number of addresses
			if len(addresses) != len(tc.expectedIPs) {
				t.Errorf("Expected %d addresses, got %d", len(tc.expectedIPs), len(addresses))
			}

			// Check that all expected IPs are in the result
			for expectedIP := range tc.expectedIPs {
				if _, exists := addresses[expectedIP]; !exists {
					t.Errorf("Expected address %s not found in result", expectedIP)
				}
			}

			// Check that the minimum TTL is as expected
			if ttl != tc.expectedMinTTL {
				t.Errorf("Expected min TTL %v, got %v", tc.expectedMinTTL, ttl)
			}

			// Check for IPv4 and IPv6 addresses specifically
			hasIPv4 := false
			hasIPv6 := false
			for addr := range addresses {
				host, _, _ := net.SplitHostPort(addr)
				ip := net.ParseIP(strings.Trim(host, "[]"))
				if ip != nil {
					if ip.To4() != nil {
						hasIPv4 = true
					} else {
						hasIPv6 = true
					}
				}
			}

			if hasIPv4 != tc.expectedHasIPv4 {
				t.Errorf("Expected hasIPv4=%v, got %v", tc.expectedHasIPv4, hasIPv4)
			}

			if hasIPv6 != tc.expectedHasIPv6 {
				t.Errorf("Expected hasIPv6=%v, got %v", tc.expectedHasIPv6, hasIPv6)
			}
		})
	}
}

func TestSuppressCustomResolver(t *testing.T) {
	// Create a context with custom resolver
	customResolvers := []string{"dns://1.1.1.1:53"}
	ctx := context.WithValue(context.Background(), CustomResolverKey, customResolvers)

	// Before suppressing, check that we can get the resolvers
	val := ctx.Value(CustomResolverKey)
	resolvers, ok := val.([]string)
	if !ok || len(resolvers) != 1 || resolvers[0] != "dns://1.1.1.1:53" {
		t.Errorf("Failed to set up context correctly: %v", val)
	}

	// Suppress the resolvers
	suppressedCtx := suppressCustomResolver(ctx)

	// Check it's gone
	val = suppressedCtx.Value(CustomResolverKey)
	if val != nil {
		t.Errorf("Expected nil value after suppressing, got %v", val)
	}

	// Make sure original context is unmodified
	val = ctx.Value(CustomResolverKey)
	resolvers, ok = val.([]string)
	if !ok || len(resolvers) != 1 || resolvers[0] != "dns://1.1.1.1:53" {
		t.Errorf("Original context was modified: %v", val)
	}
}
