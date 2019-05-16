package test

import (
	"net"
	"sync/atomic"

	"github.com/miekg/dns"
)

// ResponseWriter is useful for writing tests. It uses some fixed values for the client. The
// remote will always be 10.240.0.1 and port 40212. The local address is always 127.0.0.1 and
// port 53.
type ResponseWriter struct {
	TCP               bool // if TCP is true we return an TCP connection instead of an UDP one.
	RemoteIP          net.IP
	writeMsgCallCount uint64
}

// LocalAddr returns the local address, 127.0.0.1:53 (UDP, TCP if t.TCP is true).
func (t *ResponseWriter) LocalAddr() net.Addr {
	ip := net.ParseIP("127.0.0.1")
	port := 53
	if t.TCP {
		return &net.TCPAddr{IP: ip, Port: port, Zone: ""}
	}
	return &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

// RemoteAddr returns the remote address, always 10.240.0.1:40212 (UDP, TCP is t.TCP is true).
func (t *ResponseWriter) RemoteAddr() net.Addr {
	var ip net.IP
	if t.RemoteIP != nil {
		ip = t.RemoteIP
	} else {
		ip = net.ParseIP("10.240.0.1")
	}

	port := 40212
	if t.TCP {
		return &net.TCPAddr{IP: ip, Port: port, Zone: ""}
	}
	return &net.UDPAddr{IP: ip, Port: port, Zone: ""}
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *ResponseWriter) WriteMsg(m *dns.Msg) error {
	atomic.AddUint64(&t.writeMsgCallCount, 1)
	return nil
}

// Write implement dns.ResponseWriter interface.
func (t *ResponseWriter) Write(buf []byte) (int, error) { return len(buf), nil }

// Close implement dns.ResponseWriter interface.
func (t *ResponseWriter) Close() error { return nil }

// TsigStatus implement dns.ResponseWriter interface.
func (t *ResponseWriter) TsigStatus() error { return nil }

// TsigTimersOnly implement dns.ResponseWriter interface.
func (t *ResponseWriter) TsigTimersOnly(bool) { return }

// Hijack implement dns.ResponseWriter interface.
func (t *ResponseWriter) Hijack() { return }

// GetWriteMsgCallCount returns the number of WriteMsg calls that were made.
func (t *ResponseWriter) GetWriteMsgCallCount() uint64 { return atomic.LoadUint64(&t.writeMsgCallCount) }

// ResetWriteMsgCallCount returns the number of WriteMsg calls that were made.
func (t *ResponseWriter) ResetWriteMsgCallCount() { atomic.StoreUint64(&t.writeMsgCallCount, 0) }

// ResponseWriter6 returns fixed client and remote address in IPv6.  The remote
// address is always fe80::42:ff:feca:4c65 and port 40212. The local address is always ::1 and port 53.
type ResponseWriter6 struct {
	ResponseWriter
}

// LocalAddr returns the local address, always ::1, port 53 (UDP, TCP is t.TCP is true).
func (t *ResponseWriter6) LocalAddr() net.Addr {
	if t.TCP {
		return &net.TCPAddr{IP: net.ParseIP("::1"), Port: 53, Zone: ""}
	}
	return &net.UDPAddr{IP: net.ParseIP("::1"), Port: 53, Zone: ""}
}

// RemoteAddr returns the remote address, always fe80::42:ff:feca:4c65 port 40212 (UDP, TCP is t.TCP is true).
func (t *ResponseWriter6) RemoteAddr() net.Addr {
	var ip net.IP
	if t.RemoteIP != nil {
		ip = t.RemoteIP
	} else {
		ip = net.ParseIP("fe80::42:ff:feca:4c65")
	}
	if t.TCP {
		return &net.TCPAddr{IP: ip, Port: 40212, Zone: ""}
	}
	return &net.UDPAddr{IP: ip, Port: 40212, Zone: ""}
}
