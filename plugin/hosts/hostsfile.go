// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is a modified version of net/hosts.go from the golang repo

package hosts

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
)

// parseIP calls discards any v6 zone info, before calling net.ParseIP.
func parseIP(addr string) net.IP {
	if i := strings.Index(addr, "%"); i >= 0 {
		// discard ipv6 zone
		addr = addr[0:i]
	}

	return net.ParseIP(addr)
}

type options struct {
	// automatically generate IP to Hostname PTR entries
	// for host entries we parse
	autoReverse bool

	// The TTL of the record we generate
	ttl uint32

	// The time between two reload of the configuration
	reload time.Duration
}

func newOptions() *options {
	return &options{
		autoReverse: true,
		ttl:         3600,
		reload:      5 * time.Second,
	}
}

// Map contains the IPv4/IPv6 and reverse mapping.
type Map struct {
	// Key for the list of literal IP addresses must be a FQDN lowercased host name.
	name4 map[string][]net.IP
	name6 map[string][]net.IP

	// Wildcard owner names (e.g. *.example.com.) map to IP addresses.
	wildName4 map[string][]net.IP
	wildName6 map[string][]net.IP

	// Key for the list of host names must be a literal IP address
	// including IPv6 address without zone identifier.
	// We don't support old-classful IP address notation.
	addr map[string][]string
}

func newMap() *Map {
	return &Map{
		name4:     make(map[string][]net.IP),
		name6:     make(map[string][]net.IP),
		wildName4: make(map[string][]net.IP),
		wildName6: make(map[string][]net.IP),
		addr:      make(map[string][]string),
	}
}

// Len returns the total number of addresses in the hostmap, this includes V4/V6 and any reverse addresses.
func (h *Map) Len() int {
	l := 0
	for _, v4 := range h.name4 {
		l += len(v4)
	}
	for _, v6 := range h.name6 {
		l += len(v6)
	}
	for _, v4 := range h.wildName4 {
		l += len(v4)
	}
	for _, v6 := range h.wildName6 {
		l += len(v6)
	}
	for _, a := range h.addr {
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
	hmap *Map

	// inline saves the hosts file that is inlined in a Corefile.
	inline *Map

	// path to the hosts file
	path string

	// mtime and size are only read and modified by a single goroutine
	mtime time.Time
	size  int64

	options *options
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
	if err != nil {
		return
	}
	h.RLock()
	size := h.size
	mtime := h.mtime
	h.RUnlock()

	if mtime.Equal(stat.ModTime()) && size == stat.Size() {
		return
	}

	newMap := h.parse(file)
	log.Debugf("Parsed hosts file into %d entries", newMap.Len())

	h.Lock()

	h.hmap = newMap
	// Update the data cache.
	h.mtime = stat.ModTime()
	h.size = stat.Size()

	hostsEntries.WithLabelValues(h.path).Set(float64(h.inline.Len() + h.hmap.Len()))
	hostsReloadTime.Set(float64(stat.ModTime().UnixNano()) / 1e9)
	h.Unlock()
}

func (h *Hostsfile) initInline(inline []string) {
	if len(inline) == 0 {
		return
	}

	h.inline = h.parse(strings.NewReader(strings.Join(inline, "\n")))
}

// maxFieldSize bounds the memory used while assembling a single field that
// spans several reads. A DNS name is at most 255 octets, so a longer field can
// never yield a usable entry and is discarded instead of being buffered.
const maxFieldSize = 1024

// Parse reads the hostsfile and populates the byName and addr maps.
//
// Lines are read with a bufio.Reader and parsed field by field as the data
// arrives, so a line of any length is handled with a fixed amount of memory
// and never aborts the parse of the entries that follow it.
func (h *Hostsfile) parse(r io.Reader) *Map {
	hmap := newMap()
	p := lineParser{h: h, hmap: hmap}

	reader := bufio.NewReader(r)
	for {
		chunk, err := reader.ReadSlice('\n')
		// The slice returned by ReadSlice is only valid until the next read,
		// so feed consumes it before looping. ErrBufferFull means the line
		// continues in the next chunk.
		p.feed(chunk, err != bufio.ErrBufferFull)
		if err == nil || err == bufio.ErrBufferFull {
			continue
		}
		if err != io.EOF {
			log.Errorf("Failed to parse hosts file %q: %v", h.path, err)
		}
		return hmap
	}
}

// lineParser turns a stream of chunks into hosts file entries. It keeps only
// the current field, so its memory use does not grow with the line length.
type lineParser struct {
	h    *Hostsfile
	hmap *Map

	field     []byte // the field being assembled, possibly spanning chunks
	oversized bool   // the current field exceeded maxFieldSize and is dropped
	index     int    // number of fields already seen on this line
	comment   bool   // the rest of this line is a comment
	addr      net.IP // address of the current line, nil if unusable
	family    int
}

// feed consumes one chunk of the current line. last reports whether the chunk
// ends the line.
func (p *lineParser) feed(chunk []byte, last bool) {
	if !p.comment {
		if i := bytes.IndexByte(chunk, '#'); i >= 0 {
			// Discard comments.
			chunk = chunk[:i]
			p.comment = true
		}
		p.scan(chunk, last || p.comment)
	}
	if last {
		p.index, p.comment, p.addr = 0, false, nil
	}
}

// scan splits a chunk into fields. A field at the end of the chunk is only
// complete if terminal is set, otherwise it continues in the next chunk.
func (p *lineParser) scan(b []byte, terminal bool) {
	for len(b) > 0 {
		i := 0
		for i < len(b) && isSpace(b[i]) {
			i++
		}
		if i > 0 {
			// Whitespace terminates the field before it.
			p.emit()
			b = b[i:]
			continue
		}
		j := 0
		for j < len(b) && !isSpace(b[j]) {
			j++
		}
		p.append(b[:j])
		b = b[j:]
	}
	if terminal {
		p.emit()
	}
}

// append extends the current field, dropping it once it grows beyond any
// length a DNS name can have.
func (p *lineParser) append(b []byte) {
	if p.oversized {
		return
	}
	if len(p.field)+len(b) > maxFieldSize {
		p.oversized = true
		p.field = p.field[:0]
		return
	}
	p.field = append(p.field, b...)
}

// emit handles a completed field. It is a no-op when no field is pending.
func (p *lineParser) emit() {
	if len(p.field) == 0 && !p.oversized {
		return
	}
	// field aliases p.field's storage, which is reused by the next append; it
	// is only read below, before any further append happens.
	field, oversized := p.field, p.oversized
	p.field, p.oversized = p.field[:0], false
	p.index++

	if p.index == 1 {
		// The first field is the address; without it the line is unusable.
		if oversized {
			return
		}
		p.addr = parseIP(string(field))
		if p.addr == nil {
			return
		}
		if p.addr.To4() != nil {
			p.family = 1
		} else {
			p.family = 2
		}
		return
	}
	if p.addr == nil || oversized {
		return
	}
	p.addName(string(field))
}

func (p *lineParser) addName(field string) {
	name := plugin.Name(field).Normalize()
	if plugin.Zones(p.h.Origins).Matches(name) == "" {
		// name is not in Origins
		return
	}
	if isWildcardName(name) {
		switch p.family {
		case 1:
			p.hmap.wildName4[name] = append(p.hmap.wildName4[name], p.addr)
		case 2:
			p.hmap.wildName6[name] = append(p.hmap.wildName6[name], p.addr)
		}
		return
	}
	switch p.family {
	case 1:
		p.hmap.name4[name] = append(p.hmap.name4[name], p.addr)
	case 2:
		p.hmap.name6[name] = append(p.hmap.name6[name], p.addr)
	default:
		return
	}
	if !p.h.options.autoReverse {
		return
	}
	key := p.addr.String()
	p.hmap.addr[key] = append(p.hmap.addr[key], name)
}

func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

func (h *Hostsfile) lookupStaticHostLocked(m, wild map[string][]net.IP, host string) []net.IP {
	if ips, ok := m[host]; ok {
		ipsCp := make([]net.IP, len(ips))
		copy(ipsCp, ips)
		return ipsCp
	}
	if pattern := replaceWithAsteriskLabel(host); pattern != "" {
		if ips, ok := wild[pattern]; ok {
			ipsCp := make([]net.IP, len(ips))
			copy(ipsCp, ips)
			return ipsCp
		}
	}
	return nil
}

func (h *Hostsfile) lookupStaticHostFamily(host string, v4 bool) []net.IP {
	host = strings.ToLower(host)

	h.RLock()
	defer h.RUnlock()

	// h.hmap and h.inline must be read under the lock: readHosts swaps h.hmap
	// under h.Lock() on every reload.
	var ip1, ip2 []net.IP
	if v4 {
		ip1 = h.lookupStaticHostLocked(h.hmap.name4, h.hmap.wildName4, host)
		ip2 = h.lookupStaticHostLocked(h.inline.name4, h.inline.wildName4, host)
	} else {
		ip1 = h.lookupStaticHostLocked(h.hmap.name6, h.hmap.wildName6, host)
		ip2 = h.lookupStaticHostLocked(h.inline.name6, h.inline.wildName6, host)
	}
	return append(ip1, ip2...)
}

// LookupStaticHostV4 looks up the IPv4 addresses for the given host from the hosts file.
func (h *Hostsfile) LookupStaticHostV4(host string) []net.IP {
	return h.lookupStaticHostFamily(host, true)
}

// LookupStaticHostV6 looks up the IPv6 addresses for the given host from the hosts file.
func (h *Hostsfile) LookupStaticHostV6(host string) []net.IP {
	return h.lookupStaticHostFamily(host, false)
}

// LookupStaticAddr looks up the hosts for the given address from the hosts file.
func (h *Hostsfile) LookupStaticAddr(addr string) []string {
	addr = parseIP(addr).String()
	if addr == "" {
		return nil
	}

	h.RLock()
	defer h.RUnlock()
	hosts1 := h.hmap.addr[addr]
	hosts2 := h.inline.addr[addr]

	if len(hosts1) == 0 && len(hosts2) == 0 {
		return nil
	}

	hostsCp := make([]string, len(hosts1)+len(hosts2))
	copy(hostsCp, hosts1)
	copy(hostsCp[len(hosts1):], hosts2)
	return hostsCp
}
