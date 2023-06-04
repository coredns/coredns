package dnsserver

import (
	"crypto/tls"
	"net"
)

// ServerQUIC represents an instance of a DNS-over-QUIC server.
type ServerQUIC struct {
	*Server
	listenAddr net.Addr
	tlsConfig  *tls.Config
}

// NewServerQUIC returns a new CoreDNS QUIC server and compiles all plugin in to it.
func NewServerQUIC(addr string, group []*Config) (*ServerQUIC, error) {
	s, err := NewServer(addr, group)
	if err != nil {
		return nil, err
	}
	// The *tls* plugin must make sure that multiple conflicting
	// TLS configuration returns an error: it can only be specified once.
	var tlsConfig *tls.Config
	for _, z := range s.zones {
		for _, conf := range z {
			// Should we error if some configs *don't* have TLS?
			tlsConfig = conf.TLSConfig
		}
	}

	return &ServerQUIC{Server: s, tlsConfig: tlsConfig}, nil
}
