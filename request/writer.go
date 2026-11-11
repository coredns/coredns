package request

import (
	"crypto/tls"
	"slices"

	"github.com/miekg/dns"
)

// ScrubWriter preserves the client's EDNS capabilities and makes replies fit the client's buffer.
type ScrubWriter struct {
	dns.ResponseWriter
	req       *dns.Msg // live request, including plugin changes to options and buffer size
	clientOPT dns.OPT  // client EDNS header, captured before plugins can change the request
}

// NewScrubWriter returns a new and initialized ScrubWriter.
func NewScrubWriter(req *dns.Msg, w dns.ResponseWriter) *ScrubWriter {
	s := &ScrubWriter{ResponseWriter: w, req: req}
	if opt := req.IsEdns0(); opt != nil {
		s.clientOPT.Hdr = opt.Hdr
	}
	return s
}

// WriteMsg overrides the default implementation of the underlying dns.ResponseWriter and calls
// scrub on the message m and will then write it to the client.
func (s *ScrubWriter) WriteMsg(m *dns.Msg) error {
	// EDNS presence and DO belong to the client, even when plugins rewrite the request.
	req := new(dns.Msg)
	if s.clientOPT.Hdr.Rrtype == dns.TypeOPT {
		opt := s.clientOPT
		if current := s.req.IsEdns0(); current != nil {
			// Keep option rewrites/reverts and buffer limits such as bufsize.
			opt.Option = current.Option
			opt.SetUDPSize(min(opt.UDPSize(), current.UDPSize()))
		}
		req.Extra = []dns.RR{&opt}
	} else {
		m.Extra = slices.DeleteFunc(m.Extra, func(rr dns.RR) bool {
			return rr.Header().Rrtype == dns.TypeOPT
		})
		if m.Rcode > 0xf {
			// Extended RCODEs cannot be encoded without an OPT record.
			m.Rcode = dns.RcodeServerFailure
		}
	}
	state := Request{Req: req, W: s.ResponseWriter}
	state.SizeAndDo(m)
	if opt := m.IsEdns0(); opt != nil {
		opt.SetDo(s.clientOPT.Do())
	}
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
