package proxy

import (
	"crypto/tls"
	"time"

	"github.com/miekg/dns"
)

// a persistConn hold the dns.Conn and the last used time.
type persistConn struct {
	c      *dns.Conn
	dialed time.Time
	used   time.Time
}

// Transport hold the persistent cache.
type Transport struct {
	avgDialTime      int64                          // kind of average time of dial time
	conns            [typeTotalCount][]*persistConn // Buckets for udp, tcp and tcp-tls.
	idleTimeout      time.Duration                  // A connection will not be reused if it has been idle for this long.
	maxConnectionAge time.Duration                  // A connection will not be reused if it has been alive for this long.
	addr             string
	tlsConfig        *tls.Config
	proxyName        string

	dial  chan string
	yield chan *persistConn
	ret   chan *persistConn
	stop  chan bool
}

func newTransport(proxyName, addr string) *Transport {
	t := &Transport{
		avgDialTime: int64(maxDialTimeout / 2),
		conns:       [typeTotalCount][]*persistConn{},
		idleTimeout: defaultIdleTimeout,
		addr:        addr,
		dial:        make(chan string),
		yield:       make(chan *persistConn),
		ret:         make(chan *persistConn),
		stop:        make(chan bool),
		proxyName:   proxyName,
	}
	return t
}

// connManager manages the persistent connection cache for UDP and TCP.
func (t *Transport) connManager() {
	for {
		select {
		case proto := <-t.dial:
			transtype := stringToTransportType(proto)
			t.ret <- t.popValidConn(transtype)

		case pc := <-t.yield:
			transtype := t.transportTypeFromConn(pc)
			t.conns[transtype] = append(t.conns[transtype], pc)

		case <-t.stop:
			for _, stack := range t.conns {
				closeConns(stack)
			}
			close(t.ret)
			return
		}
	}
}

func (t *Transport) popValidConn(transtype transportType) *persistConn {
	now := time.Now()
	for idx, pc := range t.conns[transtype] {
		if now.Sub(pc.used) > t.idleTimeout {
			continue
		}
		if t.maxConnectionAge > 0 && now.Sub(pc.dialed) > deterministicJitter(pc.dialed, t.maxConnectionAge) {
			continue
		}

		go closeConns(t.conns[transtype][:idx])
		t.conns[transtype] = t.conns[transtype][idx+1:]
		return pc
	}
	go closeConns(t.conns[transtype])
	t.conns[transtype] = nil
	return nil
}

// closeConns closes connections.
func closeConns(conns []*persistConn) {
	for _, pc := range conns {
		pc.c.Close()
	}
}

// It is hard to pin a value to this, the import thing is to no block forever, losing at cached connection is not terrible.
const yieldTimeout = 25 * time.Millisecond

// Yield returns the connection to transport for reuse.
func (t *Transport) Yield(pc *persistConn) {
	pc.used = time.Now() // update used time

	// Make this non-blocking, because in the case of a very busy forwarder we will *block* on this yield. This
	// blocks the outer go-routine and stuff will just pile up.  We timeout when the send fails to as returning
	// these connection is an optimization anyway.
	select {
	case t.yield <- pc:
		return
	case <-time.After(yieldTimeout):
		return
	}
}

// Start starts the transport's connection manager.
func (t *Transport) Start() { go t.connManager() }

// Stop stops the transport's connection manager.
func (t *Transport) Stop() { close(t.stop) }

// SetIdleTimeout sets the connection idle timeout in transport.
func (t *Transport) SetIdleTimeout(idleTimeout time.Duration) { t.idleTimeout = idleTimeout }

// SetMaxConnectionAge sets the connection max age in transport.
func (t *Transport) SetMaxConnectionAge(maxConnectionAge time.Duration) {
	t.maxConnectionAge = maxConnectionAge
}

// SetTLSConfig sets the TLS config in transport.
func (t *Transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }

// deterministicJitter returns a deterministic time.Duration in the interval [d, 2*d).
// The jitter will be the same across calls with the same time and duration.
func deterministicJitter(t time.Time, d time.Duration) time.Duration {
	ratio := float64(t.UnixNano()&(1<<16-1)) / float64(1<<16)
	return time.Duration(float64(d) * (1 + ratio))
}

const (
	defaultIdleTimeout = 10 * time.Second
	minDialTimeout     = 1 * time.Second
	maxDialTimeout     = 30 * time.Second
)
