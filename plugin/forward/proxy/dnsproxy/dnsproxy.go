package dnsproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/pkg/up"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// DNSProxy defines an upstream host.
type DNSProxy struct {
	fails uint32

	addr string

	// Connection
	transport *Transport
	forceTCP  bool
	preferUDP bool

	// health checking
	health HealthChecker
	probe  *up.Probe

	// Metrics
	m *metrics.Metrics
}

// DNSOpts defines an DNSProxy options.
type DNSOpts struct {
	TLSConfig *tls.Config
	Expire    time.Duration
	ForceTCP  bool
	PreferUDP bool
}

// New returns a new proxy.
func New(addr string, opts *DNSOpts, metrics *metrics.Metrics) *DNSProxy {
	p := &DNSProxy{
		fails:     0,
		addr:      addr,
		transport: newTransport(addr, opts.TLSConfig, opts.Expire, metrics),
		forceTCP:  opts.ForceTCP,
		preferUDP: opts.PreferUDP,
		health:    newHealthChecker(transport.DNS, metrics),
		probe:     up.New(),
		m:         metrics,
	}

	if opts.TLSConfig != nil {
		p.health.setTLSConfig(opts.TLSConfig)
	}

	return p
}

// Query selects an upstream, sends the request and waits for a response.
func (p *DNSProxy) Query(ctx context.Context, state request.Request) (*dns.Msg, error) {
	fmt.Printf("--- Query via DNS %s --- \n", p.addr)

	start := time.Now()
	proto := ""
	switch {
	case p.forceTCP: // TCP flag has precedence over UDP flag
		proto = "tcp"
	case p.preferUDP:
		proto = "udp"
	default:
		proto = state.Proto()
	}

	var (
		conn   *dns.Conn
		cached bool
		ret    *dns.Msg
		err    error
	)
	for {
		conn, cached, err = p.transport.Dial(proto)
		if err != nil {
			break
		}

		// Set buffer size correctly for this client.
		conn.UDPSize = uint16(state.Size())
		if conn.UDPSize < 512 {
			conn.UDPSize = 512
		}

		conn.SetWriteDeadline(time.Now().Add(maxTimeout))
		if err = conn.WriteMsg(state.Req); err != nil {
			conn.Close() // not giving it back
			if err == io.EOF && cached {
				continue // cached connection was closed by peer
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(readTimeout))
		ret, err = conn.ReadMsg()
		if err != nil {
			conn.Close() // not giving it back
			if err == io.EOF && cached {
				continue // cached connection was closed by peer
			}
			if ret != nil && ret.Truncated && !p.forceTCP && p.preferUDP {
				p.forceTCP = true
				continue
			}
			break
		}

		p.transport.Yield(conn)

		rc, ok := dns.RcodeToString[ret.Rcode]
		if !ok {
			rc = strconv.Itoa(ret.Rcode)
		}

		p.m.RequestCount.WithLabelValues(p.addr).Add(1)
		p.m.RcodeCount.WithLabelValues(rc, p.addr).Add(1)
		p.m.RequestDuration.WithLabelValues(p.addr).Observe(time.Since(start).Seconds())
		break
	}

	return ret, err
}

// Healthcheck kicks of a round of health checks for this proxy.
func (p *DNSProxy) Healthcheck() {
	p.probe.Do(func() error {
		return p.health.check(p)
	})
}

// Down returns true if this proxy is down, i.e. has *more* fails than maxfails.
func (p *DNSProxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// Stop stops the health checking goroutine.
func (p *DNSProxy) Stop() { p.probe.Stop() }

// Start starts the proxy's healthchecking.
func (p *DNSProxy) Start(duration time.Duration) {
	p.probe.Start(duration)
	p.transport.Start()
}

// Address returns the proxy's address.
func (p *DNSProxy) Address() string { return p.addr }

// FailsNum returns the proxy's fails number.
func (p *DNSProxy) FailsNum() *uint32 { return &p.fails }

var (
	// ErrCachedClosed means cached connection was closed by peer.
	ErrCachedClosed = errors.New("cached connection was closed by peer")
)

const (
	maxTimeout = 2 * time.Second
)
