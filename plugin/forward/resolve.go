package forward

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
)

const defaultResolveInterval = 0 * time.Second // disabled by default; hostnames resolved only at startup

// hostEntry represents a hostname-based TO address that needs DNS resolution.
type hostEntry struct {
	hostname  string // the hostname to resolve (e.g., "rbldnsd.rbldnsd.svc.cluster.local")
	port      string // port (e.g., "53", "853")
	transport string // "dns" or "tls"
	zone      string // TLS server name zone (from %zone syntax)
}

// toEntry represents a single TO address from the config, preserving order.
type toEntry struct {
	static bool      // true for IP/file-based entries
	addrs  []string  // for static: resolved by HostPortOrFile
	entry  hostEntry // for dynamic: hostname to resolve
}

// classifyToAddrs processes TO addresses in order, returning an ordered list of
// toEntries that preserves config ordering.
func classifyToAddrs(toAddrs []string) ([]toEntry, error) {
	var entries []toEntry
	for _, h := range toAddrs {
		// Try HostPortOrFile first - this handles IPs and files
		hosts, parseErr := parse.HostPortOrFile(h)
		if parseErr == nil {
			entries = append(entries, toEntry{static: true, addrs: hosts})
			continue
		}

		// Only fall through to hostname parsing if the error specifically
		// indicates the address is not an IP or file. Other errors (like
		// "no nameservers found" from file parsing) should be propagated.
		if !strings.Contains(parseErr.Error(), "not an IP address or file") {
			return nil, parseErr
		}

		// Not an IP or file - check if it's a valid hostname
		entry, ok := parseAsHostEntry(h)
		if !ok {
			return nil, fmt.Errorf("not an IP address, file, or valid domain: %q", h)
		}
		entries = append(entries, toEntry{static: false, entry: entry})
	}
	return entries, nil
}

// parseAsHostEntry attempts to parse a TO address as a hostname-based entry.
func parseAsHostEntry(h string) (hostEntry, bool) {
	cleanH, zone := splitZone(h)
	trans, host := parse.Transport(cleanH)

	// Only dns and tls transports are supported for hostname resolution
	if trans != transport.DNS && trans != transport.TLS {
		return hostEntry{}, false
	}

	hostname := host
	port := transport.Port
	if trans == transport.TLS {
		port = transport.TLSPort
	}

	// Check if there's a port
	if h2, p, err := net.SplitHostPort(host); err == nil {
		hostname = h2
		port = p
	}

	hostname = strings.Trim(hostname, "[]")

	// Validate as domain name
	if _, ok := dns.IsDomainName(hostname); !ok || hostname == "" {
		return hostEntry{}, false
	}

	// Make sure it's not actually an IP
	if net.ParseIP(hostname) != nil {
		return hostEntry{}, false
	}

	return hostEntry{
		hostname:  hostname,
		port:      port,
		transport: trans,
		zone:      zone,
	}, true
}

// expandAndDedup resolves all toEntries in order, expands hostnames to IPs,
// and deduplicates by first-seen address. Returns the deduplicated address list.
func expandAndDedup(entries []toEntry, resolvers []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, e := range entries {
		var addrs []string
		if e.static {
			addrs = e.addrs
		} else {
			resolved, err := resolveHostEntry(e.entry, resolvers)
			if err != nil {
				return nil, err
			}
			addrs = resolved
		}

		for _, addr := range addrs {
			// Normalize the address for dedup comparison
			key := normalizeAddr(addr)
			if !seen[key] {
				seen[key] = true
				result = append(result, addr)
			}
		}
	}
	return result, nil
}

// normalizeAddr extracts the canonical IP:port from an address string
// (stripping transport prefix and zone) for deduplication.
func normalizeAddr(addr string) string {
	host, _ := splitZone(addr)
	_, h := parse.Transport(host)
	return h
}

// resolveHostEntry resolves a single hostname entry and returns its addresses.
func resolveHostEntry(entry hostEntry, resolvers []string) ([]string, error) {
	ips, err := lookupHost(entry.hostname, resolvers)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %q: %v", entry.hostname, err)
	}
	var addrs []string
	for _, ip := range ips {
		addrs = append(addrs, formatResolvedAddr(ip, entry.port, entry.transport, entry.zone))
	}
	return addrs, nil
}

// formatResolvedAddr formats a resolved IP into an address string compatible
// with the proxy creation code in parseStanza.
func formatResolvedAddr(ip, port, trans, zone string) string {
	isIPv6 := strings.Contains(ip, ":")

	switch trans {
	case transport.TLS:
		if zone != "" {
			if isIPv6 {
				return transport.TLS + "://[" + ip + "%" + zone + "]:" + port
			}
			return transport.TLS + "://" + ip + "%" + zone + ":" + port
		}
		return transport.TLS + "://" + net.JoinHostPort(ip, port)
	default: // transport.DNS
		return net.JoinHostPort(ip, port)
	}
}

// lookupHost resolves a hostname to IP addresses using the specified resolvers.
// If resolvers is empty, the system resolver (/etc/resolv.conf) is used.
func lookupHost(hostname string, resolvers []string) ([]string, error) {
	if len(resolvers) == 0 {
		return systemLookup(hostname)
	}
	return dnsLookup(hostname, resolvers)
}

// systemLookup resolves using the system resolver (/etc/resolv.conf).
func systemLookup(hostname string) ([]string, error) {
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses found for %q", hostname)
	}
	return ips, nil
}

// dnsLookup resolves a hostname using specific DNS resolver addresses.
// Each resolver can be a bare IP (port 53 is assumed) or an IP:port pair.
// It tries each resolver in order until one succeeds.
func dnsLookup(hostname string, resolvers []string) ([]string, error) {
	c := new(dns.Client)
	c.ReadTimeout = 2 * time.Second
	c.WriteTimeout = 2 * time.Second

	var lastErr error

	for _, resolver := range resolvers {
		resolverAddr := resolver
		if _, _, err := net.SplitHostPort(resolver); err != nil {
			resolverAddr = net.JoinHostPort(resolver, transport.Port)
		}
		var ips []string

		// Try A records
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
		m.RecursionDesired = true

		r, _, err := c.Exchange(m, resolverAddr)
		if err != nil {
			lastErr = err
			continue
		}
		if r != nil {
			for _, ans := range r.Answer {
				if a, ok := ans.(*dns.A); ok {
					ips = append(ips, a.A.String())
				}
			}
		}

		// Also try AAAA
		m = new(dns.Msg)
		m.SetQuestion(dns.Fqdn(hostname), dns.TypeAAAA)
		m.RecursionDesired = true

		r, _, err = c.Exchange(m, resolverAddr)
		if err != nil {
			if len(ips) > 0 {
				return ips, nil // we have A records, AAAA failure is OK
			}
			lastErr = err
			continue
		}
		if r != nil {
			for _, ans := range r.Answer {
				if aaaa, ok := ans.(*dns.AAAA); ok {
					ips = append(ips, aaaa.AAAA.String())
				}
			}
		}

		if len(ips) > 0 {
			return ips, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no addresses found for %q: %v", hostname, lastErr)
	}
	return nil, fmt.Errorf("no addresses found for %q", hostname)
}

// hasDynamicEntries returns true if any toEntry is dynamic (hostname-based).
func hasDynamicEntries(entries []toEntry) bool {
	for _, e := range entries {
		if !e.static {
			return true
		}
	}
	return false
}

// startResolveLoop starts a background goroutine that periodically re-resolves
// hostname-based TO addresses and updates the proxy list.
func (f *Forward) startResolveLoop() {
	if !hasDynamicEntries(f.toEntries) || f.resolveInterval == 0 {
		return
	}

	f.stopResolve = make(chan struct{})
	go func() {
		ticker := time.NewTicker(f.resolveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				f.reResolve()
			case <-f.stopResolve:
				return
			}
		}
	}()
}

// stopResolveLoop stops the background resolution goroutine.
func (f *Forward) stopResolveLoop() {
	if f.stopResolve != nil {
		close(f.stopResolve)
	}
}

// reResolve re-resolves all hostname entries, deduplicates globally preserving
// first-seen order, and updates the proxy list if the result changed.
func (f *Forward) reResolve() {
	newAddrs, err := expandAndDedup(f.toEntries, f.resolver)
	if err != nil {
		log.Warningf("Failed to re-resolve upstream hostnames: %v", err)
		return
	}

	// Compare as sorted sets to avoid false positives from DNS response reordering
	newNormalized := make([]string, len(newAddrs))
	for i, addr := range newAddrs {
		newNormalized[i] = normalizeAddr(addr)
	}
	sort.Strings(newNormalized)

	f.proxyMu.RLock()
	changed := len(f.proxies) != len(newAddrs)
	if !changed {
		currentAddrs := make([]string, len(f.proxies))
		for i, p := range f.proxies {
			currentAddrs[i] = p.Addr()
		}
		sort.Strings(currentAddrs)
		for i := range currentAddrs {
			if currentAddrs[i] != newNormalized[i] {
				changed = true
				break
			}
		}
	}
	f.proxyMu.RUnlock()

	if !changed {
		return
	}

	log.Infof("Upstream addresses changed, updating proxies")

	// Build new proxy list
	var newProxies []*proxy.Proxy
	for _, addr := range newAddrs {
		host, zone := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.configureProxy(p, trans, zone)
		p.Start(f.hcInterval)
		newProxies = append(newProxies, p)
	}

	// Swap atomically
	f.proxyMu.Lock()
	oldProxies := f.proxies
	f.proxies = newProxies
	f.proxyMu.Unlock()

	// Stop old proxies (outside lock)
	for _, p := range oldProxies {
		p.Stop()
	}
}

// configureProxy applies the Forward instance's configuration to a proxy.
func (f *Forward) configureProxy(p *proxy.Proxy, trans, zone string) {
	if trans == transport.TLS {
		if zone != "" && f.tlsServerName == "" {
			tlsConfig := f.tlsConfig.Clone()
			tlsConfig.ServerName = zone
			p.SetTLSConfig(tlsConfig)
		} else {
			p.SetTLSConfig(f.tlsConfig)
		}
	}
	p.SetExpire(f.expire)
	p.SetMaxAge(f.maxAge)
	p.SetMaxIdleConns(f.maxIdleConns)
	p.GetHealthchecker().SetRecursionDesired(f.opts.HCRecursionDesired)
	if f.opts.ForceTCP && trans != transport.TLS {
		p.GetHealthchecker().SetTCPTransport()
	}
	p.GetHealthchecker().SetDomain(f.opts.HCDomain)
}
