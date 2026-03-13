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

const defaultResolveInterval = 30 * time.Second

// hostEntry represents a hostname-based TO address that needs DNS resolution.
type hostEntry struct {
	hostname  string // the hostname to resolve (e.g., "rbldnsd.rbldnsd.svc.cluster.local")
	port      string // port (e.g., "53", "853")
	transport string // "dns" or "tls"
	zone      string // TLS server name zone (from %zone syntax)
}

// classifyToAddrs separates TO addresses into IP/file-based addresses (already
// validated by HostPortOrFile) and hostname entries that need DNS resolution.
func classifyToAddrs(toAddrs []string) (ipOrFile []string, hostnames []hostEntry, err error) {
	for _, h := range toAddrs {
		// Try HostPortOrFile first - this handles IPs and files
		hosts, parseErr := parse.HostPortOrFile(h)
		if parseErr == nil {
			ipOrFile = append(ipOrFile, hosts...)
			continue
		}

		// Only fall through to hostname parsing if the error specifically
		// indicates the address is not an IP or file. Other errors (like
		// "no nameservers found" from file parsing) should be propagated.
		if !strings.Contains(parseErr.Error(), "not an IP address or file") {
			return nil, nil, parseErr
		}

		// Not an IP or file - check if it's a valid hostname
		entry, ok := parseAsHostEntry(h)
		if !ok {
			return nil, nil, fmt.Errorf("not an IP address, file, or valid domain: %q", h)
		}
		hostnames = append(hostnames, entry)
	}
	return ipOrFile, hostnames, nil
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

// resolveHostEntries resolves all hostname entries to addresses suitable for
// proxy creation. The returned addresses are in the same format as HostPortOrFile
// output (e.g., "10.0.0.1:53" or "tls://10.0.0.1:853").
func resolveHostEntries(entries []hostEntry, resolvers []string) ([]string, error) {
	var addrs []string
	for _, e := range entries {
		ips, err := lookupHost(e.hostname, resolvers)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %q: %v", e.hostname, err)
		}
		for _, ip := range ips {
			addrs = append(addrs, formatResolvedAddr(ip, e.port, e.transport, e.zone))
		}
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

// startResolveLoop starts a background goroutine that periodically re-resolves
// hostname-based TO addresses and updates the proxy list.
func (f *Forward) startResolveLoop() {
	if len(f.hostEntries) == 0 {
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

// reResolve re-resolves hostname entries and updates proxies if IPs changed.
func (f *Forward) reResolve() {
	resolvedAddrs, err := resolveHostEntries(f.hostEntries, f.resolver)
	if err != nil {
		log.Warningf("Failed to re-resolve upstream hostnames: %v", err)
		return
	}

	// Check if addresses changed
	f.proxyMu.RLock()
	currentDynamic := f.proxies[f.staticCount:]
	changed := len(currentDynamic) != len(resolvedAddrs)
	if !changed {
		currentAddrs := make([]string, len(currentDynamic))
		for i, p := range currentDynamic {
			currentAddrs[i] = p.Addr()
		}
		sort.Strings(currentAddrs)

		newAddrs := make([]string, len(resolvedAddrs))
		for i, addr := range resolvedAddrs {
			host, _ := splitZone(addr)
			_, h := parse.Transport(host)
			newAddrs[i] = h
		}
		sort.Strings(newAddrs)

		for i := range currentAddrs {
			if currentAddrs[i] != newAddrs[i] {
				changed = true
				break
			}
		}
	}
	f.proxyMu.RUnlock()

	if !changed {
		return
	}

	log.Infof("Upstream hostname addresses changed, updating proxies")

	// Create new dynamic proxies
	var newDynamic []*proxy.Proxy
	for _, addr := range resolvedAddrs {
		host, zone := splitZone(addr)
		trans, h := parse.Transport(host)
		p := proxy.NewProxy("forward", h, trans)
		f.configureProxy(p, trans, zone)
		p.Start(f.hcInterval)
		newDynamic = append(newDynamic, p)
	}

	// Swap proxies atomically
	f.proxyMu.Lock()
	oldDynamic := make([]*proxy.Proxy, len(f.proxies)-f.staticCount)
	copy(oldDynamic, f.proxies[f.staticCount:])
	newProxies := make([]*proxy.Proxy, f.staticCount+len(newDynamic))
	copy(newProxies, f.proxies[:f.staticCount])
	copy(newProxies[f.staticCount:], newDynamic)
	f.proxies = newProxies
	f.proxyMu.Unlock()

	// Stop old dynamic proxies (outside lock)
	for _, p := range oldDynamic {
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
