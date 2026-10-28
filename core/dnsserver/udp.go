package dnsserver

import (
	"net"
	"strconv"

	cproxyproto "github.com/coredns/coredns/plugin/pkg/proxyproto"

	"github.com/miekg/dns"
	"github.com/pires/go-proxyproto"
)

// withUDPDestination preserves the destination of each packet on wildcard UDP
// listeners. Keep the original listener in Caddy: ServePacket also receives
// sockets inherited during a reload and must install the adapter there.
func withUDPDestination(p net.PacketConn) net.PacketConn {
	if proxy, ok := p.(*cproxyproto.PacketConn); ok {
		wrapped := *proxy
		wrapped.PacketConn = withUDPDestination(proxy.PacketConn)
		if proxy.ConnPolicy != nil {
			wrapped.ConnPolicy = func(options proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
				// Policy callbacks must keep seeing the native transport peer.
				if packet, ok := options.Upstream.(*udpDestinationAddr); ok {
					options.Upstream = packet.UDPAddr
				}
				return proxy.ConnPolicy(options)
			}
		}
		return &wrapped
	}
	conn, ok := p.(*net.UDPConn)
	if !ok {
		return p
	}
	local := conn.LocalAddr().(*net.UDPAddr)
	// Bound sockets already report their local address. In particular, avoid
	// sending packet control messages on bound Darwin sockets (miekg/dns#724).
	if !local.IP.IsUnspecified() || !enableUDPPacketInfo(conn) {
		return p
	}
	return &udpDestinationConn{UDPConn: conn, local: *local}
}

type udpDestinationConn struct {
	*net.UDPConn
	local net.UDPAddr
}

type udpPacketInfo struct {
	dst     net.IP
	ifIndex int
}

// Keep normal route selection for unicast replies. Scoped IPv6 addresses also
// need their interface; a global address must not force the incoming route.
func (p udpPacketInfo) replyInterface() int {
	if p.dst.To4() == nil && p.dst.IsLinkLocalUnicast() {
		return p.ifIndex
	}
	return 0
}

// udpDestinationAddr travels through dns.Server's generic PacketConn path,
// which retains the ReadFrom address for both RemoteAddr and WriteTo. All
// packet information belongs to this request, not to the shared listener.
type udpDestinationAddr struct {
	*net.UDPAddr
	local net.UDPAddr
	info  udpPacketInfo
}

func (a *udpDestinationAddr) LocalAddr() net.Addr { return &a.local }

func (c *udpDestinationConn) ReadFrom(b []byte) (int, net.Addr, error) {
	oob := make([]byte, udpOOBSize)
	n, oobn, flags, peer, err := c.ReadMsgUDP(b, oob)
	if err != nil {
		return n, peer, err
	}
	info := parseUDPPacketInfo(oob[:oobn], flags)
	if info.dst == nil {
		return n, peer, nil
	}
	if ip4 := info.dst.To4(); ip4 != nil {
		info.dst = ip4
	}
	local := net.UDPAddr{IP: info.dst, Port: c.local.Port}
	if info.dst.To4() == nil && (info.dst.IsLinkLocalUnicast() || info.dst.IsLinkLocalMulticast() || info.dst.IsInterfaceLocalMulticast()) && info.ifIndex > 0 {
		local.Zone = strconv.Itoa(info.ifIndex)
	}
	return n, &udpDestinationAddr{UDPAddr: peer, local: local, info: info}, nil
}

func (c *udpDestinationConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if packet, ok := addr.(*udpDestinationAddr); ok {
		n, _, err := c.WriteMsgUDP(b, udpSourceControl(packet.info), packet.UDPAddr)
		if err == nil {
			// Datagram writes are atomic. Windows can report zero bytes for a
			// successful synchronous WSASendMsg on supported Go versions.
			return len(b), nil
		}
		return n, err
	}
	return c.UDPConn.WriteTo(b, addr)
}

type udpResponseWriter struct {
	dns.ResponseWriter
	local, remote net.Addr
}

func (w *udpResponseWriter) LocalAddr() net.Addr  { return w.local }
func (w *udpResponseWriter) RemoteAddr() net.Addr { return w.remote }

func withUDPLocalAddr(w dns.ResponseWriter) dns.ResponseWriter {
	remote := w.RemoteAddr()
	packet, ok := remote.(interface{ LocalAddr() net.Addr })
	if !ok {
		return w
	}
	local := packet.LocalAddr()
	if local == nil {
		return w
	}
	// Preserve the native peer type on plain UDP and the existing proxyproto
	// address (including its client address and TLVs) on PROXY listeners.
	if addr, ok := remote.(*udpDestinationAddr); ok {
		remote = addr.UDPAddr
	}
	return &udpResponseWriter{ResponseWriter: w, local: local, remote: remote}
}
