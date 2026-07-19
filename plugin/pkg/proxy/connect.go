// Package proxy implements a forwarding proxy with connection caching.
// It manages a pool of upstream connections (UDP and TCP) to reuse them for subsequent requests,
// reducing latency and handshake overhead. It supports in-band health checking.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/doh"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	ErrTransportStopped = "proxy: transport stopped"
)

var ErrInvalidRequest = errors.New("proxy: invalid request")

// limitTimeout is a utility function to auto-tune timeout values
// average observed time is moved towards the last observed delay moderated by a weight
// next timeout to use will be the double of the computed average, limited by min and max frame.
func limitTimeout(currentAvg *int64, minValue time.Duration, maxValue time.Duration) time.Duration {
	rt := time.Duration(atomic.LoadInt64(currentAvg))
	if rt < minValue {
		return minValue
	}
	if rt < maxValue/2 {
		return 2 * rt
	}
	return maxValue
}

func averageTimeout(currentAvg *int64, observedDuration time.Duration, weight int64) {
	dt := time.Duration(atomic.LoadInt64(currentAvg))
	atomic.AddInt64(currentAvg, int64(observedDuration-dt)/weight)
}

func (t *Transport) dialTimeout() time.Duration {
	return limitTimeout(&t.avgDialTime, minDialTimeout, maxDialTimeout)
}

func (t *Transport) updateDialTimeout(newDialTime time.Duration) {
	averageTimeout(&t.avgDialTime, newDialTime, cumulativeAvgWeight)
}

// Dial dials the address configured in transport, potentially reusing a connection or creating a new one.
func (t *Transport) Dial(proto string) (*persistConn, bool, error) {
	// If tls has been configured; use it.
	if t.tlsConfig != nil {
		proto = "tcp-tls"
	}

	// Check if transport is stopped before attempting to dial
	select {
	case <-t.stop:
		return nil, false, errors.New(ErrTransportStopped)
	default:
	}

	transtype := stringToTransportType(proto)

	t.mu.Lock()
	// Pre-compute max-age deadline outside the loop to avoid repeated time.Now() calls.
	var maxAgeDeadline time.Time
	if t.maxAge > 0 {
		maxAgeDeadline = time.Now().Add(-t.maxAge)
	}
	// FIFO: take the oldest conn (front of slice) for source port diversity
	for len(t.conns[transtype]) > 0 {
		pc := t.conns[transtype][0]
		t.conns[transtype] = t.conns[transtype][1:]
		if time.Since(pc.used) > t.expire {
			pc.c.Close()
			continue
		}
		if !maxAgeDeadline.IsZero() && pc.created.Before(maxAgeDeadline) {
			pc.c.Close()
			continue
		}
		t.mu.Unlock()
		connCacheHitsCount.WithLabelValues(t.proxyName, t.addr, proto).Add(1)
		return pc, true, nil
	}
	t.mu.Unlock()

	connCacheMissesCount.WithLabelValues(t.proxyName, t.addr, proto).Add(1)

	reqTime := time.Now()
	timeout := t.dialTimeout()
	dialer := &net.Dialer{Timeout: timeout}

	if t.localAddress != nil {
		if proto == "udp" {
			dialer.LocalAddr = &net.UDPAddr{IP: t.localAddress}
		} else {
			dialer.LocalAddr = &net.TCPAddr{IP: t.localAddress}
		}
	}

	// pass nil tlsConfig to use system default
	client := dns.Client{Net: proto, Dialer: dialer, TLSConfig: t.tlsConfig}

	conn, err := client.Dial(t.addr)

	t.updateDialTimeout(time.Since(reqTime))
	return &persistConn{c: conn, created: time.Now()}, false, err
}

func (p *Proxy) lookupDNS(_ctx context.Context, state request.Request, opts Options) (*dns.Msg, net.Addr, string, error) {
	var proto string
	switch {
	case opts.ForceTCP: // TCP flag has precedence over UDP flag
		proto = "tcp"
	case opts.PreferUDP:
		proto = "udp"
	default:
		proto = state.Proto()
	}

	originId := state.Req.Id
	state.Req.Id = dns.Id()
	defer func() {
		state.Req.Id = originId
	}()

	var wire []byte
	if state.Req.IsTsig() == nil {
		var err error
		wire, err = state.Req.Pack()
		if err != nil {
			return nil, nil, proto, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
	}

	pc, cached, err := p.transport.Dial(proto)
	if err != nil {
		return nil, nil, proto, err
	}

	// Dial may have upgraded the transport (e.g. from udp to tcp for DoT),
	// so report the transport the dialed connection actually uses, not the
	// requested proto.
	if p.transport.transportTypeFromConn(pc) == typeUDP {
		proto = "udp"
	} else {
		proto = "tcp"
	}

	// localAddr is CoreDNS's own outbound address on the upstream socket.
	// The forward plugin reports it as the dnstap query_address (the
	// initiator) so that query_address and response_address describe the
	// two ends of the same upstream connection.
	localAddr := pc.c.LocalAddr()

	// Set buffer size correctly for this client.
	pc.c.UDPSize = max(uint16(state.Size()), 512) // #nosec G115 -- UDP size fits in uint16

	pc.c.SetWriteDeadline(time.Now().Add(maxTimeout))
	if wire != nil {
		_, err = pc.c.Write(wire)
	} else {
		err = pc.c.WriteMsg(state.Req)
	}
	if err != nil {
		pc.c.Close() // not giving it back
		if err == io.EOF && cached {
			return nil, localAddr, proto, ErrCachedClosed
		}
		return nil, localAddr, proto, err
	}

	var ret *dns.Msg
	pc.c.SetReadDeadline(time.Now().Add(p.readTimeout))
	for {
		ret, err = pc.c.ReadMsg()
		if err != nil {
			if ret != nil && (state.Req.Id == ret.Id) && p.transport.transportTypeFromConn(pc) == typeUDP && shouldTruncateResponse(err) {
				// For UDP, if the error is an overflow, we probably have an upstream misbehaving in some way.
				// (e.g. sending >512 byte responses without an eDNS0 OPT RR).
				// Instead of returning an error, return an empty response with TC bit set. This will make the
				// client retry over TCP (if that's supported) or at least receive a clean
				// error. The connection is still good so we break before the close.

				// Truncate the response.
				ret = truncateResponse(ret)
				break
			}

			pc.c.Close() // not giving it back
			if err == io.EOF && cached {
				return nil, localAddr, proto, ErrCachedClosed
			}
			// recovery the origin Id after upstream.
			if ret != nil {
				ret.Id = originId
			}
			return ret, localAddr, proto, err
		}
		// drop out-of-order responses
		if state.Req.Id == ret.Id {
			break
		}
	}
	p.transport.Yield(pc)

	return ret, localAddr, proto, nil
}

func (p *Proxy) lookupDoH(ctx context.Context, state request.Request, _ Options) (*dns.Msg, net.Addr, string, error) {
	// DoH always runs over TCP (HTTPS), regardless of the downstream
	// client's protocol.
	const proto = "tcp"

	var localAddr net.Addr
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			localAddr = info.Conn.LocalAddr()
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	req, err := doh.NewRequestWithContext(ctx, p.dohMethod, p.addr, state.Req)
	if err != nil {
		return nil, nil, proto, err
	}

	resp, err := p.transport.httpClient.Do(req)
	if err != nil {
		return nil, localAddr, proto, err
	}

	// ResponseToMsg always closes the body via defer resp.Body.Close().
	ret, err := doh.ResponseToMsg(resp)
	if err != nil {
		return nil, localAddr, proto, err
	}

	return ret, localAddr, proto, nil
}

// Connect selects an upstream, sends the request and waits for a response. It
// also returns CoreDNS's own outbound address on the upstream socket
// (localAddr) and the transport proto ("udp" or "tcp") actually used to reach
// the upstream.
func (p *Proxy) Connect(ctx context.Context, state request.Request, opts Options) (*dns.Msg, net.Addr, string, error) {
	start := time.Now()
	originId := state.Req.Id

	var (
		ret       *dns.Msg
		localAddr net.Addr
		proto     string
		err       error
	)
	switch p.protocol {
	case transport.HTTPS:
		ret, localAddr, proto, err = p.lookupDoH(ctx, state, opts)
	case transport.DNS, transport.TLS:
		ret, localAddr, proto, err = p.lookupDNS(ctx, state, opts)
	default:
		return nil, nil, "", fmt.Errorf("transport %s not supported to proxy", p.protocol)
	}
	if err != nil {
		return nil, localAddr, proto, err
	}

	// recovery the origin Id after upstream.
	ret.Id = originId

	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = strconv.Itoa(ret.Rcode)
	}

	requestDuration.WithLabelValues(p.proxyName, p.addr, rc).Observe(time.Since(start).Seconds())

	return ret, localAddr, proto, nil
}

const cumulativeAvgWeight = 4

// Function to determine if a response should be truncated.
func shouldTruncateResponse(err error) bool {
	// This is to handle a scenario in which upstream sets the TC bit, but doesn't truncate the response
	// and we get ErrBuf instead of overflow.
	if _, isDNSErr := err.(*dns.Error); isDNSErr && errors.Is(err, dns.ErrBuf) {
		return true
	} else if strings.Contains(err.Error(), "overflow") {
		return true
	}
	return false
}

// Function to return an empty response with TC (truncated) bit set.
func truncateResponse(response *dns.Msg) *dns.Msg {
	// Clear out Answer, Extra, and Ns sections
	response.Answer = nil
	response.Extra = nil
	response.Ns = nil

	// Set TC bit to indicate truncation.
	response.Truncated = true
	return response
}
