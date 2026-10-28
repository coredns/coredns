package proxyproto

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pires/go-proxyproto"
)

var (
	_ net.PacketConn = (*PacketConn)(nil)
	_ net.Addr       = (*Addr)(nil)
)

// errHeaderOnly is a sentinel used internally to signal that the datagram
// contained only a PROXY Protocol header with no DNS payload. It is never
// returned to callers of ReadFrom.
var errHeaderOnly = errors.New("header-only datagram; no payload")

// PacketConn wraps a net.PacketConn and strips PROXY Protocol v2 headers from
// incoming UDP datagrams.
//
// When UDPSessionTrackingTTL is greater than zero the connection implements
// Cloudflare Spectrum's PPv2-over-UDP behavior: the PROXY header arrives in
// the very first datagram of a session (which may carries an empty payload)
// while all subsequent datagrams carry real DNS payload without any header.
// The real source address parsed from the first datagram is cached keyed by
// the Spectrum-side remote address and applied to every headerless datagram
// that arrives from the same remote address within UDPSessionTrackingTTL.
//
// The session cache is a fixed-capacity LRU (capped at udpSessionMaxEntries)
// so that memory usage is bounded regardless of the number of distinct remote
// addresses seen.
type PacketConn struct {
	net.PacketConn
	ConnPolicy        proxyproto.ConnPolicyFunc
	ValidateHeader    proxyproto.Validator
	ReadHeaderTimeout time.Duration

	// UDPSessionTrackingTTL enables per-remote-address session state for UDP
	// when set to a positive duration. A header-only datagram (valid PPv2
	// header with or without payload) causes the parsed source address to be
	// cached for this duration. Subsequent datagrams from the same remote
	// address that carry no PPv2 header are assigned the cached source
	// address. The TTL is refreshed on every matching packet. A zero or
	// negative value disables session tracking entirely.
	UDPSessionTrackingTTL time.Duration

	// UDPSessionTrackingMaxSessions is the maximum number of concurrent UDP
	// sessions held in the LRU cache. Zero or negative means use the default
	// (udpSessionMaxEntries). Has no effect unless UDPSessionTrackingTTL is
	// positive.
	UDPSessionTrackingMaxSessions int

	// CaptureTLVs populates Addr.rawTLVs from the PPv2 header when set. Opt-in,
	// so connections without it skip the extra TLV processing.
	CaptureTLVs bool

	// sessionCache is a thread-safe expirable LRU; lazily initialized on
	// first use when UDPSessionTrackingTTL > 0.
	sessionCache *expirable.LRU[string, *proxyproto.Header]
}

func (c *PacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	for {
		n, addr, err = c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		peer := addr
		n, addr, err = c.readFrom(p[:n], addr)
		if err != nil {
			if errors.Is(err, errHeaderOnly) {
				// Header-only datagram with no DNS payload (Spectrum PPv2 UDP
				// session establishment). Silently discard and wait for the
				// next datagram.
				continue
			}
			// drop invalid packet as returning error would cause the ReadFrom caller to exit
			// which could result in DoS if an attacker sends intentional invalid packets
			clog.Warningf("dropping invalid Proxy Protocol packet from %s: %v", peer.String(), err)
			continue
		}
		return n, addr, nil
	}
}

func (c *PacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if pa, ok := addr.(*Addr); ok {
		addr = pa.u
	}
	return c.PacketConn.WriteTo(p, addr)
}

func (c *PacketConn) readFrom(p []byte, addr net.Addr) (_ int, _ net.Addr, err error) {
	var policy proxyproto.Policy
	if c.ConnPolicy != nil {
		policy, err = c.ConnPolicy(proxyproto.ConnPolicyOptions{
			Upstream:   addr,
			Downstream: c.LocalAddr(),
		})
		if err != nil {
			return 0, nil, fmt.Errorf("applying Proxy Protocol connection policy: %w", err)
		}
	}
	if policy == proxyproto.SKIP {
		return len(p), addr, nil
	}
	header, payload, err := parseProxyProtocol(p)
	if err != nil {
		return 0, nil, err
	}
	if header != nil && c.ValidateHeader != nil {
		if err := c.ValidateHeader(header); err != nil {
			return 0, nil, fmt.Errorf("validating Proxy Protocol header: %w", err)
		}
	}
	switch policy {
	case proxyproto.REJECT:
		if header != nil {
			return 0, nil, errors.New("connection rejected by Proxy Protocol connection policy")
		}
	case proxyproto.REQUIRE:
		if header == nil {
			return 0, nil, errors.New("PROXY Protocol header required but not present")
		}
		fallthrough
	case proxyproto.USE:
		if header != nil {
			if !header.Command.IsLocal() {
				addr = &Addr{u: addr, r: header.SourceAddr, rawTLVs: c.tlvsFor(header)}

				if c.UDPSessionTrackingTTL > 0 {
					// Cache the real source address for subsequent headerless datagrams.
					// Spectrum sends the header in a standalone datagram with no DNS
					// payload; refresh or insert the entry either way so that the TTL
					// resets on every header packet.
					c.storeSession(addr.(*Addr).u, header)
				}
			}

			if c.UDPSessionTrackingTTL > 0 && len(payload) == 0 {
				// Header-only datagram: no DNS payload to return; loop back
				// to read the next datagram.
				return 0, nil, errHeaderOnly
			}
		} else if c.UDPSessionTrackingTTL > 0 {
			// No header present – look for a cached header for this remote.
			if cachedHeader, ok := c.lookupSession(addr); ok {
				addr = &Addr{u: addr, r: cachedHeader.SourceAddr, rawTLVs: c.tlvsFor(cachedHeader)}
			}
		}
	default:
	}
	copy(p, payload)
	return len(payload), addr, nil
}

type Addr struct {
	u net.Addr
	r net.Addr
	// rawTLVs is the uninterpreted PPv2 TLV vector. It rides on Addr only to
	// cross the miekg/dns ResponseWriter boundary; the proxyproto plugin reads
	// it back off RemoteAddr and publishes it as request metadata.
	rawTLVs []byte
}

// NewAddr returns an Addr whose Network reflects underlying, whose String
// reflects real (the PROXY-reported client), and that carries the raw PPv2 TLV
// vector (may be nil).
func NewAddr(underlying, real net.Addr, rawTLVs []byte) *Addr {
	return &Addr{u: underlying, r: real, rawTLVs: rawTLVs}
}

func (a *Addr) Network() string {
	return a.u.Network()
}

func (a *Addr) String() string {
	return a.r.String()
}

// RawTLVs returns the raw PPv2 TLV vector carried with this address, or nil.
func (a *Addr) RawTLVs() []byte {
	return a.rawTLVs
}

// Unwrap returns the underlying real client address (a *net.TCPAddr or
// *net.UDPAddr). Callers that classify by concrete transport/family type —
// e.g. request.Proto and request.Family — unwrap through this so an Addr is
// treated as its real transport rather than an unknown net.Addr.
func (a *Addr) Unwrap() net.Addr {
	return a.r
}

// LocalAddr returns the transport packet's destination when the underlying
// address carries one. It is independent of the PROXY header's destination.
func (a *Addr) LocalAddr() net.Addr {
	if packet, ok := a.u.(interface{ LocalAddr() net.Addr }); ok {
		return packet.LocalAddr()
	}
	return nil
}

// tlvsFor returns the header's raw TLVs when TLV capture is enabled, else nil.
func (c *PacketConn) tlvsFor(header *proxyproto.Header) []byte {
	if !c.CaptureTLVs {
		return nil
	}
	return rawTLVs(header)
}

// rawTLVs returns the header's TLV vector as uninterpreted bytes, or nil if
// the header carries no TLVs. It uses the library's public API; NOOP padding
// is dropped, which does not matter to consumers that re-split the vector.
func rawTLVs(header *proxyproto.Header) []byte {
	if header == nil {
		return nil
	}
	tlvs, err := header.TLVs()
	if err != nil || len(tlvs) == 0 {
		return nil
	}
	raw, err := proxyproto.JoinTLVs(tlvs)
	if err != nil {
		return nil
	}
	return raw
}

func parseProxyProtocol(packet []byte) (*proxyproto.Header, []byte, error) {
	reader := bufio.NewReader(bytes.NewReader(packet))

	header, err := proxyproto.Read(reader)
	if err != nil {
		if errors.Is(err, proxyproto.ErrNoProxyProtocol) {
			return nil, packet, nil
		}
		return nil, nil, fmt.Errorf("parsing Proxy Protocol header (packet size: %d): %w", len(packet), err)
	}

	if header.Version != 2 {
		return nil, nil, fmt.Errorf("unsupported Proxy Protocol version %d (only v2 supported for UDP)", header.Version)
	}

	_, _, ok := header.UDPAddrs()
	if !ok {
		return nil, nil, fmt.Errorf("PROXY Protocol header is not UDP type (transport protocol: 0x%x)", header.TransportProtocol)
	}

	headerLen := len(packet) - reader.Buffered()
	if headerLen < 0 || headerLen > len(packet) {
		return nil, nil, fmt.Errorf("invalid header length: %d", headerLen)
	}

	payload := packet[headerLen:]
	return header, payload, nil
}
