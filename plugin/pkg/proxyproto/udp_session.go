package proxyproto

import (
	"net"
	"sync"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pires/go-proxyproto"
)

// udpSessionMaxEntries is the default maximum number of concurrent UDP
// sessions that the LRU cache will track. When the cache is full the
// least-recently-used entry is evicted.
const udpSessionMaxEntries = 10_240

// sessionInitMu serializes lazy initialization of PacketConn.sessionCache.
var sessionInitMu sync.Mutex

// ensureSessionCache lazily creates the expirable LRU if it hasn't been
// created yet. The expirable.LRU itself is thread-safe once constructed.
func (c *PacketConn) ensureSessionCache() {
	if c.sessionCache != nil {
		return
	}
	sessionInitMu.Lock()
	defer sessionInitMu.Unlock()
	if c.sessionCache != nil {
		return // double-check after acquiring lock
	}
	cap := c.UDPSessionTrackingMaxSessions
	if cap <= 0 {
		cap = udpSessionMaxEntries
	}
	c.sessionCache = expirable.NewLRU[string, *proxyproto.Header](cap, nil, c.UDPSessionTrackingTTL)
}

// storeSession inserts or refreshes the session entry for remoteAddr.
// Calling Add on an existing key resets its TTL.
func (c *PacketConn) storeSession(remoteAddr net.Addr, header *proxyproto.Header) {
	c.ensureSessionCache()
	c.sessionCache.Add(sessionKey(remoteAddr), header)
}

// lookupSession returns the cached source address for remoteAddr, if one
// exists and has not expired. Looking up a key refreshes its TTL by
// re-adding it.
func (c *PacketConn) lookupSession(remoteAddr net.Addr) (*proxyproto.Header, bool) {
	if c.sessionCache == nil {
		return nil, false
	}
	key := sessionKey(remoteAddr)
	header, ok := c.sessionCache.Get(key)
	if !ok {
		return nil, false
	}
	// Refresh TTL by re-adding.
	c.sessionCache.Add(key, header)
	return header, true
}

func sessionKey(addr net.Addr) string {
	return addr.Network() + "://" + addr.String()
}
