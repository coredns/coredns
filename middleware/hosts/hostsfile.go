// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hosts

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const cacheMaxAge = 5 * time.Second

func parseLiteralIP(addr string) string {
	if i := strings.Index(addr, "%"); i >= 0 {
		// discard ipv6 zone
		addr = addr[0:i]
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return ""
	}

	return ip.String()
}

func absDomainName(b []byte) string {
	hasDots := false
	for _, x := range b {
		if x == '.' {
			hasDots = true
			break
		}
	}
	if hasDots && b[len(b)-1] != '.' {
		b = append(b, '.')
	}
	return string(b)
}

func lowerASCIIBytes(x []byte) {
	for i, b := range x {
		if 'A' <= b && b <= 'Z' {
			x[i] += 'a' - 'A'
		}
	}
}

// hosts contains known host entries.
type Hostsfile struct {
	sync.Mutex

	// Key for the list of literal IP addresses must be a host
	// name. It would be part of DNS labels, a FQDN or an absolute
	// FQDN.
	// For now the key is converted to lower case for convenience.
	byName map[string][]string

	// Key for the list of host names must be a literal IP address
	// including IPv6 address with zone identifier.
	// We don't support old-classful IP address notation.
	byAddr map[string][]string

	expire time.Time
	path   string
	mtime  time.Time
	size   int64
}

func (h *Hostsfile) ReadHosts() {
	now := time.Now()

	if now.Before(h.expire) && len(h.byName) > 0 {
		return
	}
	stat, err := os.Stat(h.path)
	if err == nil && h.mtime.Equal(stat.ModTime()) && h.size == stat.Size() {
		h.expire = now.Add(cacheMaxAge)
		return
	}

	hs := make(map[string][]string)
	is := make(map[string][]string)
	var file *os.File
	if file, _ = os.Open(h.path); file == nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if i := bytes.Index(line, []byte{'#'}); i >= 0 {
			// Discard comments.
			line = line[0:i]
		}
		f := bytes.Fields(line)
		if len(f) < 2 {
			continue
		}
		addr := parseLiteralIP(string(f[0]))
		if addr == "" {
			continue
		}
		for i := 1; i < len(f); i++ {
			name := absDomainName(f[i])
			h := []byte(f[i])
			lowerASCIIBytes(h)
			key := absDomainName(h)
			hs[key] = append(hs[key], addr)
			is[addr] = append(is[addr], name)
		}
	}
	// Update the data cache.
	h.expire = now.Add(cacheMaxAge)
	h.byName = hs
	h.byAddr = is
	h.mtime = stat.ModTime()
	h.size = stat.Size()
}

// lookupStaticHost looks up the addresses for the given host from /etc/hosts.
func (h *Hostsfile) LookupStaticHost(host string) []string {
	h.Lock()
	defer h.Unlock()
	h.ReadHosts()
	if len(h.byName) != 0 {
		// TODO(jbd,bradfitz): avoid this alloc if host is already all lowercase?
		// or linear scan the byName map if it's small enough?
		lowerHost := []byte(host)
		lowerASCIIBytes(lowerHost)
		if ips, ok := h.byName[absDomainName(lowerHost)]; ok {
			ipsCp := make([]string, len(ips))
			copy(ipsCp, ips)
			return ipsCp
		}
	}
	return nil
}

// lookupStaticAddr looks up the hosts for the given address from /etc/hosts.
func (h *Hostsfile) LookupStaticAddr(addr string) []string {
	h.Lock()
	defer h.Unlock()
	h.ReadHosts()
	addr = parseLiteralIP(addr)
	if addr == "" {
		return nil
	}
	if len(h.byAddr) != 0 {
		if hosts, ok := h.byAddr[addr]; ok {
			hostsCp := make([]string, len(hosts))
			copy(hostsCp, hosts)
			return hostsCp
		}
	}
	return nil
}

func (h *Hostsfile) Names() []string {
	h.Lock()
	defer h.Unlock()
	names := []string{}
	for name := range h.byName {
		names = append(names, name)
	}
	return names
}
