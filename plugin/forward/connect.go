// Package forward implements a forwarding proxy. It caches an upstream net.Conn for some time, so if the same
// client returns the upstream's Conn will be precached. Depending on how you benchmark this looks to be
// 50% faster than just opening a new connection for every client. It works with UDP and TCP and uses
// inband healthchecking.
package forward

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
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

func networkIP(network string, ip net.IP) net.Addr {
	switch network {
	case "tcp", "tcp-tls":
		return &net.TCPAddr{IP: ip}
	default:
		return &net.UDPAddr{IP: ip}
	}
}

func dialTimeoutLocal(network, address string, local net.IP, timeout time.Duration) (conn *dns.Conn, err error) {
	client := dns.Client{Net: network, Dialer: &net.Dialer{Timeout: timeout, LocalAddr: networkIP(network, local)}}
	return client.Dial(address)
}

func dialTimeoutLocalWithTLS(network, address string, local net.IP, tlsConfig *tls.Config, timeout time.Duration) (conn *dns.Conn, err error) {
	if !strings.HasSuffix(network, "-tls") {
		network += "-tls"
	}
	client := dns.Client{Net: network, Dialer: &net.Dialer{Timeout: timeout, LocalAddr: networkIP(network, local)}, TLSConfig: tlsConfig}
	return client.Dial(address)
}

// Dial dials the address configured in transport, potentially reusing a connection or creating a new one.
func (t *Transport) Dial(proto string) (*persistConn, bool, error) {
	// If tls has been configured; use it.
	if t.tlsConfig != nil {
		proto = "tcp-tls"
	}

	t.dial <- proto
	pc := <-t.ret

	if pc != nil {
		ConnCacheHitsCount.WithLabelValues(t.addr, proto).Add(1)
		return pc, true, nil
	}
	ConnCacheMissesCount.WithLabelValues(t.addr, proto).Add(1)

	reqTime := time.Now()
	timeout := t.dialTimeout()
	if proto == "tcp-tls" {
		conn, err := dialTimeoutLocalWithTLS("tcp", t.addr, t.local, t.tlsConfig, timeout)
		t.updateDialTimeout(time.Since(reqTime))
		return &persistConn{c: conn}, false, err
	}
	conn, err := dialTimeoutLocal(proto, t.addr, t.local, timeout)
	t.updateDialTimeout(time.Since(reqTime))
	return &persistConn{c: conn}, false, err
}

// Connect selects an upstream, sends the request and waits for a response.
func (p *Proxy) Connect(ctx context.Context, state request.Request, opts options) (*dns.Msg, error) {
	start := time.Now()

	proto := ""
	switch {
	case opts.forceTCP: // TCP flag has precedence over UDP flag
		proto = "tcp"
	case opts.preferUDP:
		proto = "udp"
	default:
		proto = state.Proto()
	}

	pc, cached, err := p.transport.Dial(proto)
	if err != nil {
		return nil, err
	}

	// Set buffer size correctly for this client.
	pc.c.UDPSize = uint16(state.Size())
	if pc.c.UDPSize < 512 {
		pc.c.UDPSize = 512
	}

	pc.c.SetWriteDeadline(time.Now().Add(maxTimeout))
	// records the origin Id before upstream.
	originId := state.Req.Id
	state.Req.Id = dns.Id()
	defer func() {
		state.Req.Id = originId
	}()

	if err := pc.c.WriteMsg(state.Req); err != nil {
		pc.c.Close() // not giving it back
		if err == io.EOF && cached {
			return nil, ErrCachedClosed
		}
		return nil, err
	}

	var ret *dns.Msg
	pc.c.SetReadDeadline(time.Now().Add(readTimeout))
	for {
		ret, err = pc.c.ReadMsg()
		if err != nil {
			pc.c.Close() // not giving it back
			if err == io.EOF && cached {
				return nil, ErrCachedClosed
			}
			// recovery the origin Id after upstream.
			if ret != nil {
				ret.Id = originId
			}
			return ret, err
		}
		// drop out-of-order responses
		if state.Req.Id == ret.Id {
			break
		}
	}
	// recovery the origin Id after upstream.
	ret.Id = originId

	p.transport.Yield(pc)

	rc, ok := dns.RcodeToString[ret.Rcode]
	if !ok {
		rc = strconv.Itoa(ret.Rcode)
	}

	RequestCount.WithLabelValues(p.addr).Add(1)
	RcodeCount.WithLabelValues(rc, p.addr).Add(1)
	RequestDuration.WithLabelValues(p.addr, rc).Observe(time.Since(start).Seconds())

	return ret, nil
}

const cumulativeAvgWeight = 4
