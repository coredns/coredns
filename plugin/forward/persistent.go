package forward

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/miekg/dns"
)

// a persistConn hold the dns.Conn and the last used time.
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

// transport hold the persistent cache.
type transport struct {
	avgDialTime int64                     // kind of average time of dial time
	conns       map[string][]*persistConn //  Buckets for udp, tcp and tcp-tls.
	expire      time.Duration             // After this duration a connection is expired.
	addr        string
	tlsConfig   *tls.Config

	dial  chan string
	yield chan *persistConn
	ret   chan *persistConn
	stop  chan bool
}

func newTransport(addr string) *transport {
	t := &transport{
		avgDialTime: int64(defaultDialTimeout / 2),
		conns:       make(map[string][]*persistConn),
		expire:      defaultExpire,
		addr:        addr,
		dial:        make(chan string),
		yield:       make(chan *persistConn),
		ret:         make(chan *persistConn),
		stop:        make(chan bool),
	}
	return t
}

// len returns the number of connection, used for metrics. Can only be safely
// used inside connManager() because of data races.
func (t *transport) len() int {
	l := 0
	for _, conns := range t.conns {
		l += len(conns)
	}
	return l
}

// connManagers manages the persistent connection cache for UDP and TCP.
func (t *transport) connManager() {
Wait:
	for {
		select {
		case proto := <-t.dial:
			// take the last used conn - complexity O(1)
			if stack := t.conns[proto]; len(stack) > 0 {
				pc := stack[len(stack)-1]
				if time.Since(pc.used) < t.expire {
					// Found one, remove from pool and return this conn.
					t.conns[proto] = stack[:len(stack)-1]
					t.ret <- pc
					continue Wait
				}
			}
			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len()))

			t.ret <- nil

		case pc := <-t.yield:

			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len() + 1))

			// no proto here, infer from config and conn
			if _, ok := pc.c.Conn.(*net.UDPConn); ok {
				t.conns["udp"] = append(t.conns["udp"], pc)
				continue Wait
			}

			if t.tlsConfig == nil {
				t.conns["tcp"] = append(t.conns["tcp"], pc)
				continue Wait
			}

			t.conns["tcp-tls"] = append(t.conns["tcp-tls"], pc)

		case <-t.stop:
			t.clean()
			close(t.ret)
			return
		}
	}
}

func (t *transport) clean() {
	for _, stack := range t.conns {
		for _, pc := range stack {
			pc.c.Close()
		}
	}
}

// Yield return the connection to transport for reuse.
func (t *transport) Yield(c *persistConn) { t.yield <- c }

// Start starts the transport's connection manager.
func (t *transport) Start() { go t.connManager() }

// Stop stops the transport's connection manager.
func (t *transport) Stop() { close(t.stop) }

// SetExpire sets the connection expire time in transport.
func (t *transport) SetExpire(expire time.Duration) { t.expire = expire }

// SetTLSConfig sets the TLS config in transport.
func (t *transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }

const (
	defaultExpire      = 10 * time.Second
	minDialTimeout     = 100 * time.Millisecond
	maxDialTimeout     = 30 * time.Second
	defaultDialTimeout = 30 * time.Second
)
