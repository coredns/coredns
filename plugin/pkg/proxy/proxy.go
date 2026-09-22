package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/pkg/up"
)

// Proxy defines an upstream host.
type Proxy struct {
	fails     uint32
	addr      string
	proxyName string

	transport *Transport
	doq       *doqTransport
	protocol  string

	dohMethod string
	dohHost   string

	readTimeout time.Duration

	// health checking
	probe  *up.Probe
	health HealthChecker
}

// NewProxy returns a new proxy.
func NewProxy(proxyName, addr, protocol string) *Proxy {
	p := &Proxy{
		addr:        addr,
		fails:       0,
		probe:       up.New(),
		readTimeout: 2 * time.Second,
		transport:   newTransport(proxyName, addr),
		protocol:    protocol,
		dohMethod:   http.MethodPost,
		dohHost:     "",
		health:      NewHealthChecker(proxyName, protocol, true, "."),
		proxyName:   proxyName,
	}
	if protocol == transport.QUIC {
		p.doq = newDoQTransport(proxyName, addr)
	}

	runtime.SetFinalizer(p, (*Proxy).finalizer)
	return p
}

func (p *Proxy) Addr() string { return p.addr }

// SetTLSConfig sets the TLS config in the lower p.transport and in the healthchecking client.
func (p *Proxy) SetTLSConfig(cfg *tls.Config) {
	p.transport.SetTLSConfig(cfg)
	if p.doq != nil {
		p.doq.setTLSConfig(cfg)
	}
	if p.health != nil {
		p.health.SetTLSConfig(cfg)
	}
	if p.transport.httpClient != nil {
		p.transport.httpClient.Transport.(*http.Transport).TLSClientConfig = cfg
	}
}

// SetExpire sets the expire duration in the lower p.transport.
func (p *Proxy) SetExpire(expire time.Duration) {
	p.transport.SetExpire(expire)
	if p.doq != nil {
		p.doq.setExpire(expire)
	}
}

// SetMaxAge sets the maximum connection lifetime in the lower p.transport.
// A value of 0 (default) disables max-age.
func (p *Proxy) SetMaxAge(maxAge time.Duration) {
	p.transport.SetMaxAge(maxAge)
	if p.doq != nil {
		p.doq.setMaxAge(maxAge)
	}
}

// SetMaxIdleConns sets the maximum idle connections per transport type.
// A value of 0 means unlimited (default).
func (p *Proxy) SetMaxIdleConns(n int) {
	p.transport.SetMaxIdleConns(n)
	if p.transport.httpClient != nil {
		p.transport.httpClient.Transport.(*http.Transport).MaxIdleConns = n
		p.transport.httpClient.Transport.(*http.Transport).MaxIdleConnsPerHost = n
	}
}

func (p *Proxy) SetHTTPClient(client *http.Client) {
	p.transport.httpClient = client
}

func (p *Proxy) SetDOHRequestOptions(method string) {
	p.dohMethod = method
}

func (p *Proxy) SetDOHHost(host string) {
	p.dohHost = host
}

func (p *Proxy) DoHHost() string { return p.dohHost }

func (p *Proxy) GetHealthchecker() HealthChecker {
	return p.health
}

func (p *Proxy) GetTransport() *Transport {
	return p.transport
}

func (p *Proxy) Fails() uint32 {
	return atomic.LoadUint32(&p.fails)
}

// Healthcheck kicks of a round of health checks for this proxy.
func (p *Proxy) Healthcheck() {
	if p.health == nil {
		log.Warning("No healthchecker")
		return
	}

	p.probe.Do(func() error {
		return p.health.Check(p)
	})
}

// Down returns true if this proxy is down, i.e. has *more* fails than maxfails.
func (p *Proxy) Down(maxfails uint32) bool {
	if maxfails == 0 {
		return false
	}

	fails := atomic.LoadUint32(&p.fails)
	return fails > maxfails
}

// Stop stops health checking and closes the DoQ transport, when configured.
func (p *Proxy) Stop() {
	p.probe.Stop()
	if p.doq != nil {
		p.doq.stopTransport()
	}
}

func (p *Proxy) finalizer() {
	if p.doq != nil {
		p.doq.stopTransport()
		return
	}
	p.transport.Stop()
}

// Start starts the proxy's healthchecking.
func (p *Proxy) Start(duration time.Duration) {
	p.probe.Start(duration)
	if p.doq != nil {
		p.doq.start()
		return
	}
	p.transport.Start()
}

func (p *Proxy) SetReadTimeout(duration time.Duration) {
	p.readTimeout = duration
	if p.doq != nil {
		p.doq.setReadTimeout(duration)
	}
}

// incrementFails increments the number of fails safely.
func (p *Proxy) incrementFails() {
	curVal := atomic.LoadUint32(&p.fails)
	if curVal > curVal+1 {
		// overflow occurred, do not update the counter again
		return
	}
	atomic.AddUint32(&p.fails, 1)
}

// SetLocalAddress sets the local address for the proxy, used as the source address for outbound connections.
func (p *Proxy) SetLocalAddress(addr net.IP) {
	p.transport.SetLocalAddress(addr)
	if p.doq != nil {
		p.doq.setLocalAddress(addr)
	}
	if p.transport.httpClient != nil {
		httpTransport := p.transport.httpClient.Transport.(*http.Transport)
		if addr == nil {
			httpTransport.DialContext = nil
			return
		}
		dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: addr}}
		httpTransport.DialContext = dialer.DialContext
	}
}

const (
	maxTimeout = 2 * time.Second
)
