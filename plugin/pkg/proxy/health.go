package proxy

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
)

// HealthChecker checks the upstream health.
type HealthChecker interface {
	Check(*Proxy) error
	SetTLSConfig(*tls.Config)
	GetTLSConfig() *tls.Config
	SetRecursionDesired(bool)
	GetRecursionDesired() bool
	SetDomain(domain string)
	GetDomain() string
	SetTCPTransport()
	GetReadTimeout() time.Duration
	SetReadTimeout(time.Duration)
	GetWriteTimeout() time.Duration
	SetWriteTimeout(time.Duration)
	SetLocalAddress(net.IP)
	GetLocalAddress() net.IP
}

// dnsHc is a health checker for a DNS endpoint (DNS, and DoT).
type dnsHc struct {
	c                *dns.Client
	recursionDesired bool
	domain           string

	proxyName string

	localAddress net.IP
}

// NewHealthChecker returns a new HealthChecker based on transport.
func NewHealthChecker(proxyName, trans string, recursionDesired bool, domain string) HealthChecker {
	switch trans {
	case transport.DNS, transport.TLS:
		c := new(dns.Client)
		c.Net = "udp"
		c.ReadTimeout = 1 * time.Second
		c.WriteTimeout = 1 * time.Second

		return &dnsHc{
			c:                c,
			recursionDesired: recursionDesired,
			domain:           domain,
			proxyName:        proxyName,
		}
	}

	log.Warningf("No healthchecker for transport %q", trans)
	return nil
}

func (h *dnsHc) SetTLSConfig(cfg *tls.Config) {
	h.c.Net = "tcp-tls"
	h.c.TLSConfig = cfg
	// update the dialer accordingly with the protocol changed
	h.setDialer()
}

func (h *dnsHc) GetTLSConfig() *tls.Config {
	return h.c.TLSConfig
}

func (h *dnsHc) SetRecursionDesired(recursionDesired bool) {
	h.recursionDesired = recursionDesired
}
func (h *dnsHc) GetRecursionDesired() bool {
	return h.recursionDesired
}

func (h *dnsHc) SetDomain(domain string) {
	h.domain = domain
}
func (h *dnsHc) GetDomain() string {
	return h.domain
}

func (h *dnsHc) SetTCPTransport() {
	h.c.Net = "tcp"
	// update the dialer accordingly with the protocol changed
	h.setDialer()
}

func (h *dnsHc) GetReadTimeout() time.Duration {
	return h.c.ReadTimeout
}

func (h *dnsHc) SetReadTimeout(t time.Duration) {
	h.c.ReadTimeout = t
}

func (h *dnsHc) GetWriteTimeout() time.Duration {
	return h.c.WriteTimeout
}

func (h *dnsHc) SetWriteTimeout(t time.Duration) {
	h.c.WriteTimeout = t
}

// For HC, we send to . IN NS +[no]rec message to the upstream. Dial timeouts and empty
// replies are considered fails, basically anything else constitutes a healthy upstream.

// Check is used as the up.Func in the up.Probe.
func (h *dnsHc) Check(p *Proxy) error {
	err := h.send(p.addr)
	if err != nil {
		healthcheckFailureCount.WithLabelValues(p.proxyName, p.addr).Add(1)
		p.incrementFails()
		return err
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}

func (h *dnsHc) send(addr string) error {
	ping := new(dns.Msg)
	ping.SetQuestion(h.domain, dns.TypeNS)
	ping.RecursionDesired = h.recursionDesired

	m, _, err := h.c.Exchange(ping, addr)
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	if err != nil && m != nil {
		// Silly check, something sane came back.
		if m.Response || m.Opcode == dns.OpcodeQuery {
			err = nil
		}
	}

	return err
}

// SetLocalAddress sets the local address in transport.
func (h *dnsHc) SetLocalAddress(localAddr net.IP) {
	h.localAddress = localAddr
	h.setDialer()
}

// GetLocalAddress returns the local address in transport.
func (h *dnsHc) GetLocalAddress() net.IP {
	return h.localAddress
}

// setDialer sets the local address in the underline dialer
func (h *dnsHc) setDialer() {
	if h.c.Dialer == nil {
		h.c.Dialer = new(net.Dialer)
	}
	if h.c.Net == "udp" {
		h.c.Dialer.LocalAddr = &net.UDPAddr{IP: h.localAddress}
	} else {
		h.c.Dialer.LocalAddr = &net.TCPAddr{IP: h.localAddress}
	}
}
