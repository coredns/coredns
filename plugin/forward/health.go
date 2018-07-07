package forward

import (
	"crypto/tls"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// HealthChecker checks the upstream health.
type HealthChecker interface {
	Check(*Proxy) error
	SetTLSConfig(*tls.Config)
}

type hc struct{ c *dns.Client }

// NewHealthChecker returns a new HealthChecker based on protocol.
func NewHealthChecker(protocol int) HealthChecker {
	switch protocol {
	case DNS, TLS:
		c := new(dns.Client)
		c.Net = "udp"
		c.ReadTimeout = 1 * time.Second
		c.WriteTimeout = 1 * time.Second

		return &hc{c: c}
	}

	return nil
}

func (h *hc) SetTLSConfig(cfg *tls.Config) {
	h.c.Net = "tcp-tls"
	h.c.TLSConfig = cfg
}

// For HC we send to . IN NS +norec message to the upstream. Dial timeouts and empty
// replies are considered fails, basically anything else constitutes a healthy upstream.

// Check is used as the up.Func in the up.Probe.
func (h *hc) Check(p *Proxy) error {
	err := h.send(p.addr)
	if err != nil {
		HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
		atomic.AddUint32(&p.fails, 1)
		return err
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}

func (h *hc) send(addr string) error {
	ping := new(dns.Msg)
	ping.SetQuestion(".", dns.TypeNS)

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
