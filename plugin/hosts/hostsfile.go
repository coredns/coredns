// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is a modified version of net/hosts.go from the golang repo

package hosts

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
)

// The crypto library starts at 1, so this is safe
const noEncoding crypto.Hash = 0

func parseLiteralIP(addr string) net.IP {
	if i := strings.Index(addr, "%"); i >= 0 {
		// discard ipv6 zone
		addr = addr[0:i]
	}

	return net.ParseIP(addr)
}

func absDomainName(b string) string {
	return plugin.Name(b).Normalize()
}

type lookupOptions struct {
	// automatically generate IP to Hostname PTR entries
	// for host entries we parse
	autoReverse bool

	// Encoding to apply before comparing the hostname with the
	// data in the hostmap, allowing to use hexencoded hashes
	// as hostname (so a reader can not reverse engineer what
	// hosts are being intercepted)
	encoding crypto.Hash
}

type hostsMap struct {
	// Key for the list of literal IP addresses must be a host
	// name. It would be part of DNS labels, a FQDN or an absolute
	// FQDN.
	// For now the key is converted to lower case for convenience.
	byNameV4 map[string][]net.IP
	byNameV6 map[string][]net.IP

	// Key for the list of host names must be a literal IP address
	// including IPv6 address with zone identifier.
	// We don't support old-classful IP address notation.
	byAddr map[string][]string

	options lookupOptions
}

func newHostsMap(options lookupOptions) *hostsMap {
	return &hostsMap{
		byNameV4: make(map[string][]net.IP),
		byNameV6: make(map[string][]net.IP),
		byAddr:   make(map[string][]string),
		options: lookupOptions{
			autoReverse: options.autoReverse,
			encoding:    options.encoding,
		},
	}
}

// Len returns the total number of addresses in the hostmap, this includes
// V4/V6 and any reverse addresses.
func (h *hostsMap) Len() int {
	l := 0
	for _, v4 := range h.byNameV4 {
		l += len(v4)
	}
	for _, v6 := range h.byNameV6 {
		l += len(v6)
	}
	for _, a := range h.byAddr {
		l += len(a)
	}
	return l
}

// Hostsfile contains known host entries.
type Hostsfile struct {
	sync.RWMutex

	// list of zones we are authoritative for
	Origins []string

	// hosts maps for lookups
	hmap *hostsMap

	// inline saves the hosts file that is inlined in a Corefile.
	// We need a copy here as we want to use it to initialize the maps for parse.
	inline *hostsMap

	// path to the hosts file
	path string

	// mtime and size are only read and modified by a single goroutine
	mtime time.Time
	size  int64

	// lookupOptions to apply when looking up hostname
	options lookupOptions
}

// readHosts determines if the cached data needs to be updated based on the size and modification time of the hostsfile.
func (h *Hostsfile) readHosts() {
	file, err := os.Open(h.path)
	if err != nil {
		// We already log a warning if the file doesn't exist or can't be opened on setup. No need to return the error here.
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err == nil && h.mtime.Equal(stat.ModTime()) && h.size == stat.Size() {
		return
	}

	newMap := h.parse(file, h.inline, h.options)
	log.Debugf("Parsed hosts file into %d entries", newMap.Len())

	h.Lock()

	h.hmap = newMap
	// Update the data cache.
	h.mtime = stat.ModTime()
	h.size = stat.Size()

	h.Unlock()
}

func (h *Hostsfile) initInline(inline []string, options lookupOptions) {
	if len(inline) == 0 {
		return
	}

	hmap := newHostsMap(options)
	h.inline = h.parse(strings.NewReader(strings.Join(inline, "\n")), hmap, options)
	*h.hmap = *h.inline
}

// Parse reads the hostsfile and populates the byName and byAddr maps.
func (h *Hostsfile) parse(r io.Reader, override *hostsMap, options lookupOptions) *hostsMap {
	hmap := newHostsMap(options)

	scanner := bufio.NewScanner(r)
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
		if addr == nil {
			continue
		}
		ver := ipVersion(string(f[0]))
		for i := 1; i < len(f); i++ {
			name := absDomainName(string(f[i]))
			if plugin.Zones(h.Origins).Matches(name) == "" {
				// name is not in Origins
				continue
			}
			switch ver {
			case 4:
				hmap.byNameV4[name] = append(hmap.byNameV4[name], addr)
			case 6:
				hmap.byNameV6[name] = append(hmap.byNameV6[name], addr)
			default:
				continue
			}
			if !options.autoReverse || options.encoding != noEncoding {
				continue
			}
			hmap.byAddr[addr.String()] = append(hmap.byAddr[addr.String()], name)
		}
	}

	if override == nil {
		return hmap
	}

	for name := range override.byNameV4 {
		hmap.byNameV4[name] = append(hmap.byNameV4[name], override.byNameV4[name]...)
	}
	for name := range override.byNameV4 {
		hmap.byNameV6[name] = append(hmap.byNameV6[name], override.byNameV6[name]...)
	}

	for addr := range override.byAddr {
		hmap.byAddr[addr] = append(hmap.byAddr[addr], override.byAddr[addr]...)
	}

	return hmap
}

// ipVersion returns what IP version was used textually
func ipVersion(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.':
			return 4
		case ':':
			return 6
		}
	}
	return 0
}

// LookupStaticHostV4 looks up the IPv4 addresses for the given host from the hosts file.
func (h *Hostsfile) LookupStaticHostV4(host string) []net.IP {
	h.RLock()
	defer h.RUnlock()
	if len(h.hmap.byNameV4) != 0 {
		switch h.options.encoding {
		case noEncoding:
			// we are out
		case crypto.MD5:
			hashed := md5.Sum([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV4[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA1:
			hashed := sha1.Sum([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV4[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA256:
			hashed := sha256.Sum256([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV4[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA512:
			hashed := sha512.Sum512([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV4[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		}
		if ips, ok := h.hmap.byNameV4[absDomainName(host)]; ok {
			ipsCp := make([]net.IP, len(ips))
			copy(ipsCp, ips)
			return ipsCp
		}
	}
	return nil
}

// LookupStaticHostV6 looks up the IPv6 addresses for the given host from the hosts file.
func (h *Hostsfile) LookupStaticHostV6(host string) []net.IP {
	h.RLock()
	defer h.RUnlock()
	if len(h.hmap.byNameV6) != 0 {
		switch h.options.encoding {
		case noEncoding:
			// we are out
		case crypto.MD5:
			hashed := md5.Sum([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV6[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA1:
			hashed := sha1.Sum([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV6[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA256:
			hashed := sha256.Sum256([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV6[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		case crypto.SHA512:
			hashed := sha512.Sum512([]byte(host))
			hexstr := hex.EncodeToString(hashed[:])
			if ips, ok := h.hmap.byNameV6[absDomainName(hexstr)]; ok {
				ipsCp := make([]net.IP, len(ips))
				copy(ipsCp, ips)
				return ipsCp
			}
		}
		if ips, ok := h.hmap.byNameV6[absDomainName(host)]; ok {
			ipsCp := make([]net.IP, len(ips))
			copy(ipsCp, ips)
			return ipsCp
		}
	}
	return nil
}

// LookupStaticAddr looks up the hosts for the given address from the hosts file.
func (h *Hostsfile) LookupStaticAddr(addr string) []string {
	h.RLock()
	defer h.RUnlock()
	addr = parseLiteralIP(addr).String()
	if addr == "" {
		return nil
	}
	if len(h.hmap.byAddr) != 0 {
		if hosts, ok := h.hmap.byAddr[addr]; ok {
			hostsCp := make([]string, len(hosts))
			copy(hostsCp, hosts)
			return hostsCp
		}
	}
	return nil
}
