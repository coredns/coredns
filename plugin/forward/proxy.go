package forward

import (
	"crypto/tls"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/up"

	"github.com/miekg/dns"
)

// Proxy defines an upstream host.
type Proxy struct {
	// Put in the beginning of this struct because of alignment on ARM (for instance).
	avgRtt int64
	fails  uint32

	addr      string
	client    *dns.Client
	tlsConfig *tls.Config

	// Connection caching
	expire    time.Duration
	transport *transport

	// health checking
	probe *up.Probe
}

// NewProxy returns a new proxy.
func NewProxy(addr string) *Proxy {
	return &Proxy{
		addr:      addr,
		fails:     0,
		probe:     up.New(),
		transport: newTransport(addr),
		avgRtt:    int64(timeout / 2),
		client:    dnsClient(nil),
	}
}

// dnsClient returns a client used for health checking.
func dnsClient(tlsConfig *tls.Config) *dns.Client {
	c := new(dns.Client)
	c.Net = "udp"
	// TODO(miek): this should be half of hcDuration?
	c.ReadTimeout = 1 * time.Second
	c.WriteTimeout = 1 * time.Second

	if tlsConfig != nil {
		c.Net = "tcp-tls"
		c.TLSConfig = tlsConfig
	}
	return c
}

// setTLSConfig sets the TLS config in the lower p.transport.
func (p *Proxy) setTLSConfig(cfg *tls.Config) {
	p.tlsConfig = cfg
	p.client = dnsClient(cfg)
}

// setExpire sets the expire duration in the lower p.transport.
func (p *Proxy) setExpire(expire time.Duration) { p.transport.setExpire(expire) }

// dial connects to the host in p with the configured transport.
func (p *Proxy) dial(proto string, tlsConfig *tls.Config) (*dns.Conn, bool, error) {
	return p.transport.Dial(proto, p.tlsConfig)
}

// yield returns the connection to the pool.
func (p *Proxy) yield(c *dns.Conn, proto string) { p.transport.Yield(c, proto) }

// healthcheck kicks of a round of health checks for this proxy.
func (p *Proxy) healthcheck() { p.probe.Do(p.Check) }

// Down returns true if this proxy is down, i.e. has *more* fails than maxfails.
func (p *Proxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// close stops the health checking goroutine.
func (p *Proxy) close() {
	p.probe.Stop()
	p.transport.Stop()
}

// start starts the proxy's healthchecking.
func (p *Proxy) start(duration time.Duration) { p.probe.Start(duration) }

const (
	dialTimeout = 4 * time.Second
	timeout     = 2 * time.Second
	maxTimeout  = 2 * time.Second
	minTimeout  = 10 * time.Millisecond
	hcDuration  = 500 * time.Millisecond
)
