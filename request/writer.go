package request

import (
	"crypto/tls"
	"net"

	"github.com/miekg/dns"
)

// ScrubWriter will, when writing the message, call scrub to make it fit the client's buffer.
type ScrubWriter struct {
	dns.ResponseWriter
	req *dns.Msg // original request
}

// NewScrubWriter returns a new and initialized ScrubWriter.
func NewScrubWriter(req *dns.Msg, w dns.ResponseWriter) *ScrubWriter { return &ScrubWriter{w, req} }

// WriteMsg overrides the default implementation of the underlying dns.ResponseWriter and calls
// scrub on the message m and will then write it to the client.
func (s *ScrubWriter) WriteMsg(m *dns.Msg) error {
	state := Request{Req: s.req, W: s.ResponseWriter}
	state.SizeAndDo(m)
	state.Scrub(m)
	return s.ResponseWriter.WriteMsg(m)
}

// ConnectionState forwards the TLS connection state from the wrapped
// dns.ResponseWriter, if any. Method-set promotion through the embedded
// dns.ResponseWriter does not surface ConnectionState because it belongs to
// the separate dns.ConnectionStater interface, so plugins that need TLS state
// (e.g. SNI) would otherwise lose access to it once ScrubWriter wraps the
// underlying writer.
func (s *ScrubWriter) ConnectionState() *tls.ConnectionState {
	if cs, ok := s.ResponseWriter.(dns.ConnectionStater); ok {
		return cs.ConnectionState()
	}
	return nil
}

// Proto gets the protocol used as the transport. This will be udp or tcp.
func (s *ScrubWriter) Proto() string {
	// return Write.Proto(), if it is implemented
	if protoProvider, ok := s.ResponseWriter.(interface{ Proto() string }); ok {
		return protoProvider.Proto()
	}
	if _, ok := s.ResponseWriter.RemoteAddr().(*net.UDPAddr); ok {
		return "udp"
	}
	if _, ok := s.ResponseWriter.RemoteAddr().(*net.TCPAddr); ok {
		return "tcp"
	}
	return "udp"
}
