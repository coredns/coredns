package proxyproto

import (
	"net"

	"github.com/pires/go-proxyproto"
)

// NewTLVListener wraps l with a proxyproto.Listener using connPolicy, and makes
// accepted connections surface the PPv2 TLV vector via RemoteAddr. The TLVs
// ride on the address only to cross the miekg/dns boundary; the proxyproto
// plugin reads them back and publishes them as request metadata.
func NewTLVListener(l net.Listener, connPolicy proxyproto.ConnPolicyFunc) net.Listener {
	return &TLVListener{inner: &proxyproto.Listener{Listener: l, ConnPolicy: connPolicy}}
}

// TLVListener is a net.Listener whose connections surface PPv2 TLVs via
// RemoteAddr. Construct one with NewTLVListener.
type TLVListener struct {
	inner net.Listener
}

func (l *TLVListener) Addr() net.Addr { return l.inner.Addr() }
func (l *TLVListener) Close() error   { return l.inner.Close() }

// Accept returns a connection whose RemoteAddr is a *Addr carrying the PPv2
// TLVs, when the accepted connection is a *proxyproto.Conn with a header.
func (l *TLVListener) Accept() (net.Conn, error) {
	c, err := l.inner.Accept()
	if err != nil {
		return nil, err
	}
	pc, ok := c.(*proxyproto.Conn)
	if !ok {
		return c, nil
	}
	return &tlvConn{Conn: pc, pc: pc}, nil
}

// tlvConn overrides RemoteAddr to carry the connection's PPv2 TLV vector.
type tlvConn struct {
	net.Conn
	pc *proxyproto.Conn
}

// RemoteAddr returns the real client address, wrapped in *Addr with the PPv2
// TLVs when the header carries any. ProxyHeader triggers the connection's
// one-time header processing; policy handling stays inside go-proxyproto.
func (c *tlvConn) RemoteAddr() net.Addr {
	if a := AddrFromConn(c.pc); a != nil {
		return a
	}
	return c.pc.RemoteAddr()
}

// AddrFromConn returns an *Addr carrying the connection's PPv2 TLVs, or nil if
// none are available. It first checks whether RemoteAddr already yields an
// *Addr (our tlvConn's RemoteAddr is promoted through wrappers such as
// *tls.Conn and the connection-limiting conn, which all delegate RemoteAddr),
// then falls back to unwrapping to a *proxyproto.Conn. Transports that do not
// use RemoteAddr as the capture path (e.g. DoH) call this to recover the TLVs.
func AddrFromConn(conn net.Conn) *Addr {
	if conn == nil {
		return nil
	}
	if a, ok := conn.RemoteAddr().(*Addr); ok {
		return a
	}
	pc := underlyingProxyConn(conn)
	if pc == nil {
		return nil
	}
	header := pc.ProxyHeader()
	// Discard the protocol block for LOCAL commands (PROXY protocol spec); the
	// UDP path excludes LOCAL too. Publishing LOCAL TLVs would give consumers
	// context that should have been ignored.
	if header == nil || header.Command.IsLocal() {
		return nil
	}
	raw := rawTLVs(header)
	if raw == nil {
		return nil
	}
	base := pc.RemoteAddr()
	return &Addr{u: base, r: base, rawTLVs: raw}
}

// netConner is satisfied by *tls.Conn (and similar wrappers) that expose their
// underlying connection.
type netConner interface{ NetConn() net.Conn }

// underlyingProxyConn unwraps TLS and tlvConn layers to find a *proxyproto.Conn.
func underlyingProxyConn(conn net.Conn) *proxyproto.Conn {
	for i := 0; i < 4 && conn != nil; i++ {
		switch c := conn.(type) {
		case *proxyproto.Conn:
			return c
		case *tlvConn:
			conn = c.pc
		case netConner:
			conn = c.NetConn()
		default:
			return nil
		}
	}
	return nil
}
