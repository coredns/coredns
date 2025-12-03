package proxy

import (
	"context"
	"crypto/tls"
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
}

type baseHealthChecker struct {
	recursionDesired bool
	domain           string
}

// dnsHc is a health checker for a DNS endpoint (DNS, and DoT).
type dnsHc struct {
	baseHealthChecker
	c         *dns.Client
	proxyName string
}
type dohConn struct {
	baseHealthChecker
	client       *dohDNSClient
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewHealthChecker returns a new HealthChecker based on transport.
func NewHealthChecker(proxyName, trans string, recursionDesired bool, domain string) HealthChecker {

	base := baseHealthChecker{
		recursionDesired: recursionDesired,
		domain:           domain,
	}
	switch trans {
	case transport.DNS, transport.TLS:
		c := new(dns.Client)
		c.Net = "udp"
		c.ReadTimeout = 1 * time.Second
		c.WriteTimeout = 1 * time.Second

		return &dnsHc{
			c:                 c,
			baseHealthChecker: base,
			proxyName:         proxyName,
		}
	case "doh":
		return &dohConn{
			client: &dohDNSClient{
				client: sharedHTTPClient,
				url:    "",
			},
			baseHealthChecker: base,
			readTimeout:       1 * time.Second,
			writeTimeout:      1 * time.Second,
		}
	}

	log.Warningf("No healthchecker for transport %q", trans)
	return nil
}

func (b *baseHealthChecker) SetRecursionDesired(recursionDesired bool) {
	b.recursionDesired = recursionDesired
}
func (b *baseHealthChecker) GetRecursionDesired() bool {
	return b.recursionDesired
}

func (b *baseHealthChecker) SetDomain(domain string) {
	b.domain = domain
}
func (b *baseHealthChecker) GetDomain() string {
	return b.domain
}

// createHealthCheckMessage creates a standard DNS health check query.
// It sends "<domain> IN NS" with configurable recursion to test basic DNS functionality.
// This is a lightweight query that any properly functioning DNS server should handle.
func (b *baseHealthChecker) createHealthCheckMessage() *dns.Msg {
	ping := new(dns.Msg)
	ping.SetQuestion(b.domain, dns.TypeNS)
	ping.MsgHdr.RecursionDesired = b.recursionDesired
	return ping
}

// isValidResponse performs basic DNS message validation for health checking.
// If we receive a parseable DNS message (either a response or query opcode)
// then the upstream is considered healthy, even if there were network I/O errors.
// This allows health checks to succeed when DNS servers respond correctly
func (b *baseHealthChecker) isValidResponse(err error, resp *dns.Msg) error {
	if err != nil && resp != nil {
		if resp.Response || resp.Opcode == dns.OpcodeQuery {
			return nil
		}
	}
	return err
}

// processHealthCheckResult handles common health check result processing.
// Updates Prometheus metrics and proxy failure counts based on health check outcome.
// Resets failure count on success, increments on failure.
func (b *baseHealthChecker) processHealthCheckResult(err error, p *Proxy) error {
	if err != nil {
		healthcheckFailureCount.WithLabelValues(p.proxyName, p.addr).Add(1)
		p.incrementFails()
		return err
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}

func (h *dnsHc) SetTLSConfig(cfg *tls.Config) {
	h.c.Net = "tcp-tls"
	h.c.TLSConfig = cfg
}

func (h *dnsHc) GetTLSConfig() *tls.Config {
	return h.c.TLSConfig
}

func (h *dnsHc) SetTCPTransport() {
	h.c.Net = "tcp"
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
	return h.processHealthCheckResult(err, p)
}

func (h *dnsHc) send(addr string) error {
	ping := h.createHealthCheckMessage()

	m, _, err := h.c.Exchange(ping, addr)
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	err = h.isValidResponse(err, m)

	return err
}

func (h *dohConn) Check(p *Proxy) error {
	if h.client.url == "" {
		h.client.url = p.transport.dohURL
	}
	err := h.sendDoH()
	return h.processHealthCheckResult(err, p)
}

func (h *dohConn) sendDoH() error {
	ping := h.createHealthCheckMessage()

	data, err := ping.Pack()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.readTimeout)
	defer cancel()
	resp, err := h.client.Query(ctx, data)
	return h.isValidResponse(err, resp)
}

// Implement all the interface methods for dohConn
func (h *dohConn) SetTLSConfig(cfg *tls.Config) {
	// No-op: DoH health checks use HTTPS with system default TLS config.
	// Custom TLS configuration not supported for DoH health checker.
}

func (h *dohConn) GetTLSConfig() *tls.Config {
	// DoH uses default system TLS config
	return nil
}

func (h *dohConn) SetTCPTransport() {
	// No-op: http.Client does not support setting Net; transport is determined by URL scheme.
}

func (h *dohConn) GetReadTimeout() time.Duration {
	return h.readTimeout
}

func (h *dohConn) SetReadTimeout(t time.Duration) {
	h.readTimeout = t
}

func (h *dohConn) GetWriteTimeout() time.Duration {
	return h.writeTimeout
}

func (h *dohConn) SetWriteTimeout(t time.Duration) {
	h.writeTimeout = t
}
