package forward

import (
	"crypto/tls"
	"time"

	"github.com/miekg/dns"
)

// a persistConn hold the dns.Conn and the last used time.
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

// connErr is used to communicate the connection manager.
type connErr struct {
	c      *dns.Conn
	proto  string
	err    error
	cached bool
}

// connReq is used to request for a new connection from the connection manager.
type connReq struct {
	proto     string
	tlsConfig *tls.Config
}

// transport hold the persistent cache.
type transport struct {
	conns  map[string][]*persistConn // Buckets for udp, tcp and tcp-tls.
	expire time.Duration             // After this duration a connection is expired.
	addr   string

	dial  chan connReq
	yield chan connErr
	ret   chan connErr

	// Aid in testing, gets length of cache in data-race safe manner.
	lenc    chan bool
	lencOut chan int

	stop chan bool
}

func newTransport(addr string) *transport {
	t := &transport{
		conns:   make(map[string][]*persistConn),
		expire:  defaultExpire,
		addr:    addr,
		dial:    make(chan connReq),
		yield:   make(chan connErr),
		ret:     make(chan connErr),
		stop:    make(chan bool),
		lenc:    make(chan bool),
		lencOut: make(chan int),
	}
	go t.connManager()
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

// Len returns the number of connections in the cache.
func (t *transport) Len() int {
	t.lenc <- true
	l := <-t.lencOut
	return l
}

// connManagers manages the persistent connection cache for UDP and TCP.
func (t *transport) connManager() {

Wait:
	for {
		select {
		case req := <-t.dial:
			proto := req.proto
			// Yes O(n), shouldn't put millions in here. We walk all connection until we find the first one that is usuable.
			i := 0
			for i = 0; i < len(t.conns[proto]); i++ {
				pc := t.conns[proto][i]
				if time.Since(pc.used) < t.expire {
					// Found one, remove from pool and return this conn.
					t.conns[proto] = t.conns[proto][i+1:]
					t.ret <- connErr{pc.c, proto, nil, true}
					continue Wait
				}
				// This conn has expired. Close it.
				pc.c.Close()
			}

			// Not conns were found. Connect to the upstream to create one.
			t.conns[proto] = t.conns[proto][i:]
			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len()))

			go func() {
				if proto != "tcp-tls" {
					c, err := dns.DialTimeout(proto, t.addr, dialTimeout)
					t.ret <- connErr{c, proto, err, false}
					return
				}

				c, err := dns.DialTimeoutWithTLS("tcp", t.addr, req.tlsConfig, dialTimeout)
				t.ret <- connErr{c, proto, err, false}
			}()

		case conn := <-t.yield:

			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len() + 1))

			proto := conn.proto
			t.conns[proto] = append(t.conns[proto], &persistConn{conn.c, time.Now()})

		case <-t.stop:
			return

		case <-t.lenc:
			l := 0
			for _, conns := range t.conns {
				l += len(conns)
			}
			t.lencOut <- l
		}
	}
}

// Dial dials the address configured in transport, potentially reusing a connection or creating a new one.
func (t *transport) Dial(proto string, tlsConfig *tls.Config) (*dns.Conn, bool, error) {
	t.dial <- connReq{proto, tlsConfig}
	c := <-t.ret
	return c.c, c.cached, c.err
}

// Yield return the connection to transport for reuse.
func (t *transport) Yield(c *dns.Conn, proto string) {
	t.yield <- connErr{c, proto, nil, false}
}

// Stop stops the transport's connection manager.
func (t *transport) Stop() { t.stop <- true }

// setExpire sets the connection expire time in transport.
func (t *transport) setExpire(expire time.Duration) { t.expire = expire }

const defaultExpire = 10 * time.Second
