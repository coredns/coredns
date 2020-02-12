package fanout

import (
	"crypto/tls"
	"github.com/miekg/dns"
	"time"
)

// Health represents health checker for remote DNS server
type Health interface {
	SetTLSConfig(*tls.Config)
	Check() error
}

// NewHealth creates new health  with specific addr and network
func NewHealth(addr, net string) Health {
	return &health{
		addr: addr,
		c: &dns.Client{
			Net:          net,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
		},
	}
}

type health struct {
	c    *dns.Client
	addr string
}

// SetTLSConfigs sets tls config
func (h *health) SetTLSConfig(cfg *tls.Config) {
	h.c.Net = tcptlc
	h.c.TLSConfig = cfg
}

// Checks that remote DNS server is alive
func (h *health) Check() error {
	ping := new(dns.Msg)
	ping.SetQuestion(".", dns.TypeNS)
	m, _, err := h.c.Exchange(ping, h.addr)
	if err != nil && m != nil {
		if m.Response || m.Opcode == dns.OpcodeQuery {
			err = nil
		} else {
			HealthcheckFailureCount.WithLabelValues(h.addr).Add(1)
		}
	}
	return nil
}
