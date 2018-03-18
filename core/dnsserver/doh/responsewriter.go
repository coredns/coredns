package doh

import (
	"net"
	"strconv"

	"github.com/miekg/dns"
)

// ResponseWriter is a response writer that captures the result and will
// not write anything to the client. This allows us to capture the result
// and layer it upon HTTP/2.
type ResponseWriter struct {
	dns.ResponseWriter
	raddr net.Addr
	laddr net.Addr

	Msg *dns.Msg
}

// NewResponseWriter returns a new response writer.
func NewResponseWriter(raddr string, laddr net.Addr) *ResponseWriter {
	r := &ResponseWriter{laddr: laddr}

	h, p, _ := net.SplitHostPort(raddr)
	pi, _ := strconv.Atoi(p)

	ip := net.ParseIP(h)
	r.raddr = &net.TCPAddr{IP: ip, Port: pi}

	return r
}

// RemoteAddr returns the remote address.
func (r *ResponseWriter) RemoteAddr() net.Addr { return r.raddr }

// LocalAddr returns the local address.
func (r *ResponseWriter) LocalAddr() net.Addr { return r.laddr }

// WriteMsg implement dns.ResponseWriter interface.
func (r *ResponseWriter) WriteMsg(m *dns.Msg) error { r.Msg = m; return nil }

// Write implement dns.ResponseWriter interface.
func (r *ResponseWriter) Write(buf []byte) (int, error) { return len(buf), nil }

// Close implement dns.ResponseWriter interface.
func (r *ResponseWriter) Close() error { return nil }

// TsigStatus implement dns.ResponseWriter interface.
func (r *ResponseWriter) TsigStatus() error { return nil }

// TsigTimersOnly implement dns.ResponseWriter interface.
func (r *ResponseWriter) TsigTimersOnly(bool) { return }

// Hijack implement dns.ResponseWriter interface.
func (r *ResponseWriter) Hijack() { return }
