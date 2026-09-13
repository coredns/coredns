package sazu

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"

	"github.com/miekg/dns"
)

// rawKey correlates a captured request's raw bytes with the later
// ServeDNS call for that same request. DNS has no request-correlation ID
// beyond the 16-bit message ID, which is only unique per source address
// at any given moment -- exactly the scope SIG(0) verification needs it
// at, since verification happens synchronously while handling that same
// request.
type rawKey struct {
	addr string
	id   uint16
}

type capturedEntry struct {
	raw     []byte
	expires time.Time
}

// RawCapture stashes the exact wire bytes of inbound UDP or TCP
// requests, keyed by source address and DNS message ID, so a plugin's
// ServeDNS -- which only ever receives a parsed *dns.Msg -- can retrieve
// the literal bytes a client sent, for byte-exact SIG(0) (RFC 2931)
// verification that a re-encoding of the parsed message cannot guarantee
// reproduces. Wired in via DecorateReaderFunc, installable as either
// core/dnsserver.Config.UDPDecorateReaderFunc or .TCPDecorateReaderFunc
// (the same instance and the same decorator function work for both,
// since entries are keyed by address + message ID regardless of
// transport) -- see UDPDecorateReaderFunc's doc comment for why this
// exists instead of a second, separate listener.
//
// Entries are single-use: Take deletes them. An entry nobody ever claims
// (a malformed request, or a request for a zone/plugin that doesn't use
// this at all) is bounded two ways: a short TTL, checked lazily on Take
// so no background goroutine is needed, and a hard cap on the number of
// pending entries, past which the oldest unclaimed entry is evicted to
// admit the new one.
//
// Note that this decorator sees every UDP packet on whatever listener
// it's installed on, not just the ones a SAZU zone will end up handling:
// that per-packet cost (parsing a 2-byte ID, one small copy, one map
// insert under a mutex) is the real price of byte-exact access without a
// second listener. It is not a new category of cost -- miekg/dns's own
// server already pays a comparable per-packet cost for every request
// whenever any TSIG secret is configured at all, regardless of whether
// that specific request uses TSIG.
type RawCapture struct {
	ttl      time.Duration
	capacity int

	mu      sync.Mutex
	entries map[rawKey]capturedEntry
	order   []rawKey // insertion order, oldest first, for capacity eviction
}

// NewRawCapture returns a RawCapture holding at most capacity unclaimed
// entries at once, each expiring after ttl if never claimed. A few
// seconds of ttl is plenty: Take is expected to run within the same
// request's ServeDNS call, microseconds to milliseconds after Put.
func NewRawCapture(ttl time.Duration, capacity int) *RawCapture {
	return &RawCapture{
		ttl:      ttl,
		capacity: capacity,
		entries:  make(map[rawKey]capturedEntry, capacity),
	}
}

// Put records raw as the exact bytes received from addr. Safe for
// concurrent use.
func (c *RawCapture) Put(addr net.Addr, raw []byte) {
	if len(raw) < 2 {
		return // too short to even contain a message ID
	}
	id := binary.BigEndian.Uint16(raw[0:2])
	key := rawKey{addr: addr.String(), id: id}
	cp := append([]byte(nil), raw...)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.capacity {
		c.evictOldestLocked()
	}
	c.entries[key] = capturedEntry{raw: cp, expires: time.Now().Add(c.ttl)}
	c.order = append(c.order, key)
}

// evictOldestLocked drops entries from the front of c.order until one
// actually still exists in c.entries (earlier ones may already have been
// removed by Take), making room for exactly one new Put. Callers must
// hold c.mu.
func (c *RawCapture) evictOldestLocked() {
	for len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if _, ok := c.entries[oldest]; ok {
			delete(c.entries, oldest)
			return
		}
	}
}

// Take retrieves and removes the raw bytes captured for a request from
// addr with the given DNS message id, if any and not yet expired.
func (c *RawCapture) Take(addr net.Addr, id uint16) ([]byte, bool) {
	key := rawKey{addr: addr.String(), id: id}

	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	delete(c.entries, key)
	if time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.raw, true
}

// DecorateReaderFunc is installable directly as either
// core/dnsserver.Config.UDPDecorateReaderFunc or .TCPDecorateReaderFunc:
// it captures every request's raw bytes into c before handing them on
// unmodified, regardless of which transport it's installed on.
func (c *RawCapture) DecorateReaderFunc(*dnsserver.Server) dns.DecorateReader {
	return func(r dns.Reader) dns.Reader {
		pcr, _ := r.(dns.PacketConnReader)
		return &capturingReader{inner: r, innerPC: pcr, capture: c}
	}
}

// capturingReader overrides ReadTCP, ReadUDP, and ReadPacketConn -- which
// of the latter two the server actually calls depends on whether the
// underlying net.PacketConn's concrete type is *net.UDPConn or something
// more generic (e.g. under a reuseport or proxyproto wrapper); ReadTCP is
// separate again since it comes from an entirely different listener.
type capturingReader struct {
	inner   dns.Reader
	innerPC dns.PacketConnReader
	capture *RawCapture
}

func (r *capturingReader) ReadTCP(conn net.Conn, timeout time.Duration) ([]byte, error) {
	m, err := r.inner.ReadTCP(conn, timeout)
	if err == nil {
		r.capture.Put(conn.RemoteAddr(), m)
	}
	return m, err
}

func (r *capturingReader) ReadUDP(conn *net.UDPConn, timeout time.Duration) ([]byte, *dns.SessionUDP, error) {
	m, s, err := r.inner.ReadUDP(conn, timeout)
	if err == nil && s != nil {
		r.capture.Put(s.RemoteAddr(), m)
	}
	return m, s, err
}

func (r *capturingReader) ReadPacketConn(conn net.PacketConn, timeout time.Duration) ([]byte, net.Addr, error) {
	m, addr, err := r.innerPC.ReadPacketConn(conn, timeout)
	if err == nil && addr != nil {
		r.capture.Put(addr, m)
	}
	return m, addr, err
}
