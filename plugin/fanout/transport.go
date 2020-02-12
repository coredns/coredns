package fanout

import (
	"crypto/tls"
	"github.com/miekg/dns"
)

// Transport represent a solution to connect to remote DNS endpoint with specific network
type Transport interface {
	Dial(net string) (*dns.Conn, error)
	SetTLSConfig(*tls.Config)
}

// NewTransport creates new transport with address
func NewTransport(addr string) Transport {
	return &transportImpl{
		addr: addr,
	}
}

type transportImpl struct {
	tlsConfig *tls.Config
	addr      string
}

// SetTLSConfig sets tls config for transport
func (t *transportImpl) SetTLSConfig(c *tls.Config) {
	t.tlsConfig = c
}

// Dial dials the address configured in transportImpl, potentially reusing a connection or creating a new one.
func (t *transportImpl) Dial(net string) (*dns.Conn, error) {
	if t.tlsConfig != nil {
		net = tcptlc
	}
	if net == tcptlc {
		return dns.DialTimeoutWithTLS("tcp", t.addr, t.tlsConfig, defaultTimeout)
	}
	return dns.DialTimeout(net, t.addr, defaultTimeout)
}
