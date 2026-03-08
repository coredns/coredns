package proxy

import (
	"crypto/tls"
	"sort"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// a persistConn holds the dns.Conn, its creation time, and the last used time.
type persistConn struct {
	c       *dns.Conn
	created time.Time
	used    time.Time
}

// Transport hold the persistent cache.
type Transport struct {
	avgDialTime  int64                          // kind of average time of dial time
	conns        [typeTotalCount][]*persistConn // Buckets for udp, tcp and tcp-tls.
	expire       time.Duration                  // After this duration an idle connection is expired.
	maxAge       time.Duration                  // After this duration a connection is closed regardless of activity; 0 means unlimited.
	maxIdleConns int                            // Max idle connections per transport type; 0 means unlimited.
	addr         string
	tlsConfig    *tls.Config
	proxyName    string

	mu   sync.Mutex
	stop chan struct{}
}

func newTransport(proxyName, addr string) *Transport {
	t := &Transport{
		avgDialTime: int64(maxDialTimeout / 2),
		conns:       [typeTotalCount][]*persistConn{},
		expire:      defaultExpire,
		addr:        addr,
		stop:        make(chan struct{}),
		proxyName:   proxyName,
	}
	return t
}

// connManager manages the persistent connection cache for UDP and TCP.
func (t *Transport) connManager() {
	ticker := time.NewTicker(defaultExpire)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.cleanup(false)
		case <-t.stop:
			t.cleanup(true)
			return
		}
	}
}

// closeConns closes connections.
func closeConns(conns []*persistConn) {
	for _, pc := range conns {
		pc.c.Close()
	}
}

// cleanup removes connections from cache.
func (t *Transport) cleanup(all bool) {
	var toClose []*persistConn

	t.mu.Lock()
	now := time.Now()
	staleTime := now.Add(-t.expire)
	// Pre-compute max-age deadline outside the loop to avoid repeated time.Now() calls.
	var maxAgeDeadline time.Time
	if t.maxAge > 0 {
		maxAgeDeadline = now.Add(-t.maxAge)
	}
	for transtype, stack := range t.conns {
		if len(stack) == 0 {
			continue
		}
		if all {
			t.conns[transtype] = nil
			toClose = append(toClose, stack...)
			continue
		}

		// When max-age is set, use a linear scan to evaluate both the idle-timeout
		// (expire, based on last-used time) and the max-age (based on creation time).
		if t.maxAge > 0 {
			var alive []*persistConn
			for _, pc := range stack {
				if !pc.used.After(staleTime) || pc.created.Before(maxAgeDeadline) {
					toClose = append(toClose, pc)
				} else {
					alive = append(alive, pc)
				}
			}
			t.conns[transtype] = alive
			continue
		}

		// Original expire-only path: connections are sorted by "used"; use binary search.
		if stack[0].used.After(staleTime) {
			continue
		}
		// connections in stack are sorted by "used"
		good := sort.Search(len(stack), func(i int) bool {
			return stack[i].used.After(staleTime)
		})
		t.conns[transtype] = stack[good:]
		toClose = append(toClose, stack[:good]...)
	}
	t.mu.Unlock()

	// Close connections after releasing lock
	closeConns(toClose)
}

// Yield returns the connection to transport for reuse.
func (t *Transport) Yield(pc *persistConn) {
	// Check if transport is stopped before acquiring lock
	select {
	case <-t.stop:
		// If stopped, don't return to pool, just close
		pc.c.Close()
		return
	default:
	}

	pc.used = time.Now() // update used time

	t.mu.Lock()
	defer t.mu.Unlock()

	transtype := t.transportTypeFromConn(pc)

	if t.maxIdleConns > 0 && len(t.conns[transtype]) >= t.maxIdleConns {
		pc.c.Close()
		return
	}

	t.conns[transtype] = append(t.conns[transtype], pc)
}

// Start starts the transport's connection manager.
func (t *Transport) Start() { go t.connManager() }

// Stop stops the transport's connection manager.
func (t *Transport) Stop() { close(t.stop) }

// SetExpire sets the connection expire time in transport.
func (t *Transport) SetExpire(expire time.Duration) { t.expire = expire }

// SetMaxAge sets the maximum lifetime of a connection regardless of activity.
// A value of 0 (default) disables max-age and connections are only closed by expire (idle-timeout).
func (t *Transport) SetMaxAge(maxAge time.Duration) { t.maxAge = maxAge }

// SetMaxIdleConns sets the maximum idle connections per transport type.
// A value of 0 means unlimited (default).
func (t *Transport) SetMaxIdleConns(n int) { t.maxIdleConns = n }

// SetTLSConfig sets the TLS config in transport.
func (t *Transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }

// GetTLSConfig returns the TLS config in transport.
func (t *Transport) GetTLSConfig() *tls.Config { return t.tlsConfig }

const (
	defaultExpire  = 10 * time.Second
	minDialTimeout = 1 * time.Second
	maxDialTimeout = 30 * time.Second
)
