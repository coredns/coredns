package forward

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/proxy"

	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/miekg/dns"
)

type contextKey string

const (
	CustomResolverKey contextKey = "customResolver"
	https                        = "https://"
	dnsquery                     = "/dns-query"
	defaultTLSPort               = "853"
	defaultDNSPort               = "53"
	defaultDoHPort               = "443"
	veryLongTTL                  = 365 * 24 * time.Hour // one year
)

type customProxyCache struct {
	mu    sync.RWMutex
	cache map[string]cachedProxy
}

type cachedProxy struct {
	proxies    []*proxy.Proxy
	expiryTime time.Time
}

var (
	proxyCache = &customProxyCache{
		cache: make(map[string]cachedProxy),
	}
	defaultPorts = map[string]string{
		"tls": defaultTLSPort,
		"dns": defaultDNSPort,
		"doh": defaultDoHPort,
	}
	emptyAddressMap = make(map[string]struct{})
	protocolMap     = map[string]string{
		"dot":  "tls",
		"do53": "dns",
		"doh":  "doh",
	}
)

// get retrieves the cached custom proxies for a given gateway.
// It checks if the entry exists and if the TTL has not expired.
func (c *customProxyCache) get(gateway string) ([]*proxy.Proxy, bool) {
	c.mu.RLock()
	entry, valid := c.cache[gateway]
	c.mu.RUnlock()

	if valid && time.Since(entry.expiryTime) > 0 {
		valid = false
	}
	return entry.proxies, valid
}

// Helper function to remove expired cache entries
func (c *customProxyCache) removeEntry(gateway string) {
	c.mu.Lock()
	entry, exists := c.cache[gateway]
	if exists {
		delete(c.cache, gateway)
		c.mu.Unlock()
		for _, proxy := range entry.proxies {
			proxy.Stop() // Stop all proxies associated with this gateway
		}
		return
	}
	c.mu.Unlock()
}

// set caches the proxies for a given gateway with the specified expiry time
func (c *customProxyCache) set(gateway string, proxies []*proxy.Proxy, expiry time.Time) {
	// Don't cache entries with zero ttl or past expiry time
	if !expiry.After(time.Now()) {
		log.Warningf("Not caching gateway '%s' with zero TTL", gateway)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[gateway] = cachedProxy{
		proxies:    proxies,
		expiryTime: expiry,
	}
}

// getCustomProxies retrieves a list of custom DNS gateways from the context, resolves them,
// and builds proxies. It appends the default forward proxies if "Do53://infoblox.com:53" gateway is detected.
// if CustomResolverKey is not set, it returns nil and instructs to append
// the default forward proxies. If CustomResolverKey is set, but the value is not a valid list of gateways,
// or if the list is empty, it returns nil and does not append the default forward proxies.
func (f *Forward) getCustomProxies(ctx context.Context, w dns.ResponseWriter) []*proxy.Proxy {
	val := ctx.Value(CustomResolverKey)
	if val == nil {
		return nil
	}
	gatewaysList, ok := val.([]string)
	if !ok || len(gatewaysList) == 0 {
		log.Errorf("CustomResolverKey is not a valid list of gateways or is empty, returning nil and not appending default forward proxies")
		return nil
	}
	proxies := f.fetchProxies(ctx, gatewaysList, w)
	return proxies
}

func (f *Forward) fetchProxies(ctx context.Context, gateways []string, w dns.ResponseWriter) []*proxy.Proxy {
	var (
		proxies []*proxy.Proxy
	)
	for _, gateway := range gateways {
		log.Infof("Processing gateway: %s", gateway)
		customProxies := []*proxy.Proxy{}

		// Check for cached proxies
		cachedProxies, valid := proxyCache.get(gateway)
		if valid {
			proxies = append(proxies, cachedProxies...)
			continue
		}

		// Parse the gateway into transport, host, and port.
		transport, host, port, err := parseGatewayURL(gateway)
		log.Infof("Parsed gateway '%s' into transport: '%s', host: '%s', port: '%s'", gateway, transport, host, port)
		if err != nil {
			log.Errorf("Failed to parse gateway '%s' into transport, host, and port: %v", gateway, err)
			continue
		}

		// DoH handling - no IP resolution needed
		if transport == "doh" {
			dohURL := formatDoHURL(host)
			log.Infof("Formatted DoH URL for gateway '%s': %s", gateway, dohURL)
			customProxy := proxy.NewProxy(gateway, dohURL, transport)
			if customProxy == nil {
				log.Errorf("Failed to create DoH proxy for gateway '%s'", gateway)
				continue
			}
			log.Infof("created a proxy for DoH gateway '%s'", gateway)

			customProxy.Start(f.hcInterval)
			customProxies = append(customProxies, customProxy)
			// Cache with long TTL (DoH URLs don't change)
			if len(customProxies) > 0 {
				proxyCache.set(gateway, customProxies, time.Now().Add(veryLongTTL))
				proxies = append(proxies, customProxies...)
			}
			continue
		}

		// Resolve host to IPs
		ipAddresses, ttl := f.resolveFqdnToIPs(ctx, host, port, w)
		log.Infof("Resolved IP addresses for '%s': %v", host, ipAddresses)

		for _, p := range cachedProxies {
			_, found := ipAddresses[p.Addr()]
			if found {
				customProxies = append(customProxies, p)
				delete(ipAddresses, p.Addr())
			} else {
				p.Stop()
			}
		}

		// Build and start proxies
		for ipAddress := range ipAddresses {
			customProxy := f.buildProxy(host, ipAddress, transport)
			log.Infof("Building proxy for '%s': %s", host, ipAddress)
			customProxy.Start(f.hcInterval)
			customProxies = append(customProxies, customProxy)
		}
		// Cache new proxies
		if len(customProxies) > 0 {
			proxyCache.set(gateway, customProxies, time.Now().Add(time.Duration(ttl)))
			proxies = append(proxies, customProxies...)
		} else {
			proxyCache.removeEntry(gateway)
		}
	}

	if len(proxies) == 0 {
		log.Warningf("No valid custom resolvers found for gateways: %v", gateways)
	}
	log.Infof("Final list of custom proxies: %v", proxies)
	return proxies
}

// buildProxies constructs a list of Proxy objects for the given addresses (ipaddress:port).
// It sets the appropriate transport protocol, and applies TLS configuration
// if the transport is TLS.
func (f *Forward) buildProxy(host string, address string, transportProtocol string) *proxy.Proxy {
	proxy := proxy.NewProxy(address, address, transportProtocol)
	if transportProtocol == transport.TLS && net.ParseIP(host) == nil { // if host is an IP address, we don't set ServerName indicator
		log.Infof("Setting TLS config for proxy '%s' with ServerName '%s'", address, strings.TrimSuffix(host, "."))
		proxy.SetTLSConfig(&tls.Config{
			ServerName: strings.TrimSuffix(host, "."), // Server name indicator should be relative domain name
		})
	}
	return proxy
}

// resolveFqdnToIPs resolves a host (FQDN or IP) to a list of IP addresses (A and AAAA).
func (f *Forward) resolveFqdnToIPs(ctx context.Context, host, port string, w dns.ResponseWriter) (map[string]struct{}, time.Duration) {
	// If host is a valid IP, return it directly with a very long TTL.
	if ip := net.ParseIP(host); ip != nil {
		address := net.JoinHostPort(host, port)
		return map[string]struct{}{address: {}}, veryLongTTL
	}

	ctx = suppressCustomResolver(ctx)

	addresses := make(map[string]struct{})
	minTTL := veryLongTTL

	for _, qType := range []uint16{dns.TypeA, dns.TypeAAAA} {
		records, err := f.resolveGatewayName(qType, ctx, host, w)
		if err != nil {
			continue
		}
		recordTTL := extractIPsAndTTL(records, port, addresses)
		if len(addresses) > 0 && recordTTL < minTTL {
			minTTL = recordTTL
		}
	}

	if len(addresses) == 0 {
		log.Warningf("No IP addresses (IPv4 or IPv6) found for '%s'", host)
		return emptyAddressMap, 0
	}
	return addresses, minTTL
}

func (f *Forward) resolveGatewayName(dnsType uint16, ctx context.Context, host string, w dns.ResponseWriter) ([]dns.RR, error) {
	nw := nonwriter.New(w)
	msg := new(dns.Msg).SetQuestion(host, dnsType)
	_, err := f.ServeDNS(ctx, nw, msg)
	if err != nil || nw.Msg == nil {
		return nil, err
	}
	return nw.Msg.Answer, nil
}

// extractIPsAndTTL scans DNS resource records and extracts all IPv4 addresses (A records) and IPv6 addresses (AAAA records).
// It also returns the smallest TTL found among those records.
func extractIPsAndTTL(records []dns.RR, port string, addresses map[string]struct{}) time.Duration {
	minTTL := veryLongTTL
	for _, record := range records {
		var (
			ip  net.IP
			ttl uint32
		)
		switch r := record.(type) {
		case *dns.A:
			ip = r.A
			ttl = r.Hdr.Ttl
		case *dns.AAAA:
			ip = r.AAAA
			ttl = r.Hdr.Ttl
		default:
			continue
		}
		addresses[net.JoinHostPort(ip.String(), port)] = struct{}{}
		ttlDuration := time.Duration(ttl) * time.Second
		if ttlDuration < minTTL {
			minTTL = ttlDuration
		}
	}
	return minTTL
}

// Helper function to suppress CustomResolverKey from the context
func suppressCustomResolver(ctx context.Context) context.Context {
	return context.WithValue(ctx, CustomResolverKey, nil)
}

// parseGatewayURL parses a gateway URL and extracts its transport protocol (scheme),hostname, and port.
// returns error if parsing fails or transport protocol is not supported or host is invalid.
// If port is missing or invalid, default port according to transport protocol is applied.
// Returns transport, host, port
func parseGatewayURL(gateway string) (string, string, string, error) {
	parsedURL, err := url.Parse(gateway)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse gateway URL '%s': %v", gateway, err)
	}

	transport, ok := protocolMap[parsedURL.Scheme]
	if !ok {
		return "", "", "", fmt.Errorf("unsupported transport protocol '%s'", parsedURL.Scheme)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", "", "", fmt.Errorf("no host found in gateway '%s'", gateway)
	}
	if net.ParseIP(host) == nil {
		if err := validateDomainName(host, transport); err != nil { // if host is not an IP, validate it as a domain name
			return "", "", "", fmt.Errorf("invalid domain name '%s': %v", host, err)
		}
		if transport != "doh" && !strings.HasSuffix(host, ".") {
			host = host + "." // Ensure host ends with a dot for FQDN compliance
		}
	}

	port := parsedURL.Port()
	if isValidPort(port) {
		return transport, host, port, nil
	}
	port = defaultPorts[transport] // Use default port if not specified or not valid
	return transport, host, port, nil
}

// formatDoHURL ensures the DoH URL is properly formatted
func formatDoHURL(host string) string {
	return https + host + dnsquery
}

// isValidPort validates if the given port string is a number between 1 and 65535.
func isValidPort(portStr string) bool {
	portStr = strings.TrimSpace(portStr)
	portNum, err := strconv.ParseUint(portStr, 10, 16)
	return err == nil && portNum > 0
}

// validateFQDN ensures the host (domain name) adheres to FQDN rules.
// - At least one dot (not a single-label domain)
// - Single label domain is valid for doh gateways
// - Each label: 1–63 characters, alphanumerics or hyphen
// - Labels don't start/end with hyphen
// - TLD is not all-numeric
func validateDomainName(domain, transport string) error {
	domain = strings.ToLower(domain)

	if len(domain) > 255 {
		return fmt.Errorf("domain name exceeds 255 characters")
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}
	// Allow single-label domains (like localhost)
	if !strings.Contains(domain, ".") && transport == "doh" {
		if err := validateDNSLabel(domain); err != nil {
			return err
		}
		return nil
	} else if !strings.Contains(domain, ".") && transport != "doh" {
		return fmt.Errorf("domain '%s' must contain at least one dot", domain)
	}

	labels := strings.Split(domain, ".")
	for i, label := range labels {
		if err := validateDNSLabel(label); err != nil {
			return err
		}
		if unicode.IsDigit(rune(label[0])) {
			log.Warningf("Label '%s' starts with a digit", label)
		}
		// TLD cannot be numeric
		if i == len(labels)-1 && isAllNumeric(label) {
			return fmt.Errorf("TLD '%s' cannot be all numeric", label)
		}
	}
	return nil
}

func validateDNSLabel(label string) error {
	if label == "" {
		return fmt.Errorf("empty label in domain")
	}
	if len(label) > 63 {
		return fmt.Errorf("label '%s' exceeds 63 characters", label)
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return fmt.Errorf("label '%s' cannot start or end with hyphen", label)
	}
	if !isAlphaNumHyphen(label) {
		return fmt.Errorf("label '%s' contains invalid characters", label)
	}
	return nil
}

// isAlphaNumHyphen checks if a label contains only alphanumeric characters and hyphens.
func isAlphaNumHyphen(label string) bool {
	for _, c := range label {
		if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-') {
			return false
		}
	}
	return true
}

// isAllNumeric checks if a string is composed only of digits.
func isAllNumeric(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}
