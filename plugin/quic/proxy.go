package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// TODO: Make it configurable
var retries = 5

// Proxy defines an upstream host.
type Proxy struct {
	addr string

	tlsConfig  *tls.Config
	quicConfig *quic.Config

	// conn defines the connection with a DoQ proxy.
	// This connection might be closed and reopened at times.
	conn quic.Connection

	// m protects conn.
	m sync.RWMutex
}

// newProxy returns a new Proxy.
func newProxy(addr string, tlsConfig *tls.Config) *Proxy {
	if tlsConfig != nil {
		tlsConfig.NextProtos = []string{"doq"}
	}

	quicConfig := &quic.Config{
		MaxIncomingStreams:    math.MaxUint16,
		MaxIncomingUniStreams: math.MaxUint16,
	}

	// Connections are created lazily.
	return &Proxy{
		addr:       addr,
		tlsConfig:  tlsConfig,
		quicConfig: quicConfig,
	}
}

// query is a wrapper around queryQUIC that retries on expected errors.
func (p *Proxy) query(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	res, err := p.queryQUIC(ctx, req)

	for i := 0; isExpectedErr(err) && i < retries; i++ {
		res, err = p.queryQUIC(ctx, req)
	}

	return res, nil
}

// queryQUIC performs a call to the DoQ proxy.
func (p *Proxy) queryQUIC(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	start := time.Now()

	// In DoQ, one query consumes one stream.
	// The client MUST select the next available client-initiated bidirectional
	// stream for each subsequent query on a QUIC connection.
	stream, err := p.openStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open new stream to %s: %v", p.addr, err)
	}

	// When forwarding a DNS message from another transport over DoQ, the Message
	// ID MUST be set to 0.
	id := req.Id
	req.Id = 0

	buf, err := req.Pack()
	if err != nil {
		return nil, err
	}

	_, err = stream.Write(dnsserver.AddPrefix(buf))
	if err != nil {
		return nil, err
	}

	// The client MUST send the DNS query over the selected stream and
	// MUST indicate through the STREAM FIN mechanism that no further
	// data will be sent on that stream.
	_ = stream.Close()

	respBuf, err := dnsserver.ReadDOQMessage(stream)
	if err != nil {
		return nil, err
	}

	reply := dns.Msg{}
	err = reply.Unpack(respBuf)
	if err != nil {
		return nil, fmt.Errorf("unpacking response from %s: %s", p.addr, err)
	}

	// Restore the original message ID to avoid breaking compatibility
	// with other DNS protocols used in the server.
	reply.Id = id

	rc, ok := dns.RcodeToString[reply.Rcode]
	if !ok {
		rc = strconv.Itoa(reply.Rcode)
	}

	RequestCount.WithLabelValues(p.addr).Add(1)
	RcodeCount.WithLabelValues(rc, p.addr).Add(1)
	RequestDuration.WithLabelValues(p.addr).Observe(time.Since(start).Seconds())
	return &reply, nil
}

// getConnection safely gets the current connection to the Proxy.
// It's also responsible for forcing the reconnection when asked.
func (p *Proxy) getConnection(forceReconnect bool) error {
	p.m.RLock()
	conn := p.conn

	if !forceReconnect && conn != nil {
		p.m.RUnlock()
		return nil
	}

	if conn != nil {
		// Closes the previous connection just in case.
		_ = conn.CloseWithError(dnsserver.DoQCodeNoError, "")
	}

	p.m.RUnlock()
	p.m.Lock()
	defer p.m.Unlock()

	return p.createConnection()
}

func (p *Proxy) createConnection() error {
	conn, err := quic.DialAddrEarly(context.Background(), p.addr, p.tlsConfig.Clone(), p.quicConfig)
	if err != nil {
		return err
	}

	p.conn = conn
	return nil
}

// openStream opens a new QUIC stream for the specified connection.
func (p *Proxy) openStream(ctx context.Context) (quic.Stream, error) {
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(defaultTimeout))
	defer cancel()

	err := p.getConnection(false)
	if err != nil {
		return nil, fmt.Errorf("could not connect to DoQ server %s: %s", p.addr, err)
	}

	stream, err := p.conn.OpenStreamSync(ctx)
	if err == nil {
		return stream, nil
	}

	// If we are here, it means we need to reconnect to the proxy.
	err = p.getConnection(true)
	if err != nil {
		return nil, err
	}

	return p.conn.OpenStreamSync(ctx)
}

// isExpectedErr returns true if err is an expected error, likely related to
// the current implementation.
func isExpectedErr(err error) bool {
	if err == nil {
		return false
	}

	// When a connection hits the idle timeout, quic.AcceptStream() returns
	// an IdleTimeoutError. In this, case, we should just drop the connection
	// with DoQCodeNoError.
	var qIdleErr *quic.IdleTimeoutError
	if errors.As(err, &qIdleErr) {
		return true
	}

	// This error happens when we try to establish a 0-RTT connection with
	// a token the server is no more aware of. This can be reproduced by
	// restarting the QUIC server (it will clear its tokens cache). The
	// next connection attempt will return this error until the client's
	// tokens cache is purged.
	if errors.Is(err, quic.Err0RTTRejected) {
		return true
	}

	return false
}
