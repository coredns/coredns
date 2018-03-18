package doh

import (
	"net"

	"github.com/miekg/dns"
)

// ResponseWriter is a ...
type ResponseWriter struct {
	dns.ResponseWriter
	raddr net.Addr

	Msg *dns.Msg
}

func NewResponseWriter(raddr string) *ResponseWriter {

	r := new(ResponseWriter)

	ip := net.ParseIP(raddr)
	r.raddr = &net.TCPAddr{IP: ip, Port: 53}

	return r
}

// RemoteAddr returns the remote address
func (r *ResponseWriter) RemoteAddr() net.Addr { return r.raddr }

// LocalAddr returns the local address
func (r *ResponseWriter) LocalAddr() net.Addr {
	ip := net.ParseIP("127.0.0.1")
	port := 53
	return &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

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
