package proxy

import (
	"crypto/tls"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/up"
)

// Proxy defines an upstream host.
type Proxy struct {
	fails     uint32
	addr      string
	proxyName string

	transport *Transport

	readTimeout time.Duration

	// health checking
	probe  *up.Probe
	health HealthChecker
}

// NewProxy returns a new proxy.
func NewProxy(proxyName, addr, trans string) *Proxy {
	p := &Proxy{
		addr:        addr,
		fails:       0,
		probe:       up.New(),
		readTimeout: 2 * time.Second,
		transport:   newTransport(proxyName, addr),
		health:      NewHealthChecker(proxyName, trans, true, "."),
		proxyName:   proxyName,
	}

	runtime.SetFinalizer(p, (*Proxy).finalizer)
	return p
}

func (p *Proxy) Addr() string { return p.addr }

// SetTLSConfig sets the TLS config in the lower p.transport and in the healthchecking client.
func (p *Proxy) SetTLSConfig(cfg *tls.Config) {
	p.transport.SetTLSConfig(cfg)
	p.health.SetTLSConfig(cfg)
}

// SetExpire sets the expire duration in the lower p.transport.
func (p *Proxy) SetExpire(expire time.Duration) { p.transport.SetExpire(expire) }

// SetMaxIdleConns sets the maximum idle connections per transport type.
// A value of 0 means unlimited (default).
func (p *Proxy) SetMaxIdleConns(n int) { p.transport.SetMaxIdleConns(n) }

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

// Stop close stops the health checking goroutine.
func (p *Proxy) Stop()      { p.probe.Stop() }
func (p *Proxy) finalizer() { p.transport.Stop() }

// Start starts the proxy's healthchecking.
func (p *Proxy) Start(duration time.Duration) {
	p.probe.Start(duration)
	p.transport.Start()
}

func (p *Proxy) SetReadTimeout(duration time.Duration) {
	p.readTimeout = duration
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

const (
	maxTimeout = 2 * time.Second
)
