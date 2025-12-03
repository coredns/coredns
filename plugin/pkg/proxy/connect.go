// Package proxy implements a forwarding proxy. It caches an upstream net.Conn for some time, so if the same
// client returns the upstream's Conn will be precached. Depending on how you benchmark this looks to be
// 50% faster than just opening a new connection for every client. It works with UDP and TCP and uses
// inband healthchecking.
package proxy

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	ErrTransportStopped              = "proxy: transport stopped"
	ErrTransportStoppedDuringDial    = "proxy: transport stopped during dial"
	ErrTransportStoppedRetClosed     = "proxy: transport stopped, ret channel closed"
	ErrTransportStoppedDuringRetWait = "proxy: transport stopped during ret wait"
)

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
	if t.tlsConfig != nil && proto != "doh" {
		proto = "tcp-tls"
	}

	// Check if transport is stopped before attempting to dial
	select {
	case <-t.stop:
		return nil, false, errors.New(ErrTransportStopped)
	default:
	}

	// Use select to avoid blocking if connManager has stopped
	select {
	case t.dial <- proto:
		// Successfully sent dial request
	case <-t.stop:
		return nil, false, errors.New(ErrTransportStoppedDuringDial)
	}
	// Receive response with stop awareness
	select {
	case pc, ok := <-t.ret:
		if !ok {
			// ret channel was closed by connManager during stop
			return nil, false, errors.New(ErrTransportStoppedRetClosed)
		}

		if pc != nil {
			connCacheHitsCount.WithLabelValues(t.proxyName, t.addr, proto).Add(1)
			return pc, true, nil
		}
		connCacheMissesCount.WithLabelValues(t.proxyName, t.addr, proto).Add(1)

		reqTime := time.Now()
		timeout := t.dialTimeout()
		switch proto {
		case "doh":
			dohClient := getOrCreateDoHClient(t.dohURL)
			return &persistConn{dohClient: dohClient}, false, nil

		case "tcp-tls":
			conn, err := dns.DialTimeoutWithTLS("tcp", t.addr, t.tlsConfig, timeout)
			t.updateDialTimeout(time.Since(reqTime))
			return &persistConn{c: conn}, false, err

		default: // "udp", "tcp"
			conn, err := dns.DialTimeout(proto, t.addr, timeout)
			t.updateDialTimeout(time.Since(reqTime))
			return &persistConn{c: conn}, false, err
		}
	case <-t.stop:
		return nil, false, errors.New(ErrTransportStoppedDuringRetWait)
	}
}

func isDoHProxy(proxy *Proxy) bool {
	if proxy.transport != nil && proxy.transport.dohURL != "" {
		return true
	}
	addr := proxy.Addr()
	return strings.HasPrefix(addr, "doh://") ||
		strings.HasPrefix(addr, "https://") ||
		strings.Contains(addr, "/dns-query")
}

func (p *Proxy) Connect(ctx context.Context, state request.Request, opts Options) (*dns.Msg, error) {
	start := time.Now()

	// Protocol selection
	proto := ""
	switch {
	case isDoHProxy(p):
		proto = "doh"
	case opts.ForceTCP:
		proto = "tcp"
	case opts.PreferUDP:
		proto = "udp"
	default:
		proto = state.Proto()
	}
	// Dial connection
	pc, cached, err := p.transport.Dial(proto)
	if err != nil {
		return nil, err
	}

	// Record and substitute request ID
	originId := state.Req.Id
	state.Req.Id = dns.Id()
	defer func() {
		state.Req.Id = originId
	}()

	// Execute protocol-specific query
	var ret *dns.Msg
	if proto == "doh" {
		ret, err = p.handleDoHRequest(ctx, state, pc.dohClient)
	} else {
		// Validate connection for non-DoH protocols
		if pc.c == nil {
			return nil, errors.New("invalid connection for non-DoH protocol")
		}
		ret, err = p.executeNonDoHDNSQuery(state, pc)
	}

	// Common post-processing for all protocols
	return p.processQueryResult(ret, err, pc, cached, originId, start, proto)
}

func (p *Proxy) executeNonDoHDNSQuery(state request.Request, pc *persistConn) (*dns.Msg, error) {
	// Implementation for udp, tcp-tls DNS query execution
	// Set buffer size correctly for this client.
	pc.c.UDPSize = uint16(state.Size())
	if pc.c.UDPSize < 512 {
		pc.c.UDPSize = 512
	}

	pc.c.SetWriteDeadline(time.Now().Add(maxTimeout))
	if err := pc.c.WriteMsg(state.Req); err != nil {
		return nil, err
	}
	var (
		ret *dns.Msg
		err error
	)
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
				return ret, nil
			}
			return ret, err
		}
		if state.Req.Id == ret.Id {
			return ret, nil
		}
	}
}

func (p *Proxy) handleDoHRequest(ctx context.Context, state request.Request, client *dohDNSClient) (*dns.Msg, error) {
	data, err := state.Req.Pack()
	if err != nil {
		return nil, err
	}
	// Use the new DoH client's Query method
	dnsResp, err := client.Query(ctx, data)

	switch {
	case err != nil:
		log.Errorf("DoH client query failed: %v", err)
		return nil, err
	case dnsResp == nil:
		log.Errorf("DoH client returned nil response without error")
		return nil, errors.New("DoH client returned nil response")
	case dnsResp.Id != state.Req.Id:
		log.Warningf("DoH response ID mismatch: expected %d, got %d", state.Req.Id, dnsResp.Id)
		return nil, errors.New("response ID mismatch")
	default:
		return dnsResp, nil
	}
}

func (p *Proxy) processQueryResult(ret *dns.Msg, err error, pc *persistConn, cached bool, originId uint16, start time.Time, proto string) (*dns.Msg, error) {
	if err != nil {
		// Protocol-specific cleanup
		if proto != "doh" && pc.c != nil {
			pc.c.Close() // Traditional DNS connection cleanup
		}
		// DoH doesn't need explicit connection cleanup (HTTP client handles it)

		// Handle stale connections
		if err == io.EOF && cached {
			return nil, ErrCachedClosed
		}

		// Restore original ID on error
		if ret != nil {
			ret.Id = originId
		}
		return ret, err
	}

	// Success path: restore ID, yield connection, record metrics
	ret.Id = originId
	p.transport.Yield(pc)

	// Record metrics (common for all protocols)
	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = strconv.Itoa(ret.Rcode)
	}
	requestDuration.WithLabelValues(p.proxyName, p.addr, rc).Observe(time.Since(start).Seconds())
	return ret, nil
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
