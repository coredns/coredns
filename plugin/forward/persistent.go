package forward

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// a persistConn hold the dns.Conn and the last used time.
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

// Transport hold the persistent cache.
type Transport struct {
	avgDialTime int64 // kind of average time of dial time

	udpConn []*persistConn
	tcpConn []*persistConn
	tlsConn []*persistConn

	udpMu sync.RWMutex
	tcpMu sync.RWMutex
	tlsMu sync.RWMutex

	addr      string
	tlsConfig *tls.Config
}

func newTransport(addr string) *Transport {
	return &Transport{addr: addr}
}

func (t *Transport) connection(proto string) *persistConn {
	switch proto {
	case "udp":
		t.udpMu.RLock()
		lc := len(t.udpConn)
		t.udpMu.RUnlock()

		if lc == 0 {
			return nil
		}

		t.udpMu.Lock()
		pc := t.udpConn[0]
		t.udpConn = t.udpConn[1:]
		t.udpMu.Unlock()
		return pc
	case "tcp":
		t.tcpMu.RLock()
		lc := len(t.tcpConn)
		t.tcpMu.RUnlock()

		if lc == 0 {
			return nil
		}

		t.tcpMu.Lock()
		pc := t.tcpConn[0]
		t.tcpConn = t.tcpConn[1:]
		t.tcpMu.Unlock()
		return pc
	case "tcp-tls":
		t.tlsMu.RLock()
		lc := len(t.tlsConn)
		t.tlsMu.RUnlock()

		if lc == 0 {
			return nil
		}

		t.tlsMu.Lock()
		pc := t.tlsConn[0]
		t.tlsConn = t.tlsConn[1:]
		t.tlsMu.Unlock()
		return pc
	}

	return nil
}

func (t *Transport) yieldConnection(pc *persistConn) {
	// no proto here, infer from config and conn
	_, ok := pc.c.Conn.(*net.UDPConn)
	if ok {
		t.udpMu.Lock()
		t.udpConn = append(t.udpConn, pc)
		t.udpMu.Unlock()
		return
	}

	if t.tlsConfig == nil {
		t.tcpMu.Lock()
		t.tcpConn = append(t.tcpConn, pc)
		t.tcpMu.Unlock()
		return
	}

	t.tlsMu.Lock()
	t.tlsConn = append(t.tlsConn, pc)
	t.tlsMu.Unlock()
}

// SetTLSConfig sets the TLS config in transport.
func (t *Transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }

const (
	defaultExpire  = 10 * time.Second
	minDialTimeout = 1 * time.Second
	maxDialTimeout = 30 * time.Second

	// Some resolves might take quite a while, usually (cached) responses are fast. Set to 2s to give us some time to retry a different upstream.
	readTimeout = 2 * time.Second
)
