package proxyproto

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/pires/go-proxyproto"
)

var (
	_ net.PacketConn = (*PacketConn)(nil)
	_ net.Addr       = (*Addr)(nil)
)

type PacketConn struct {
	net.PacketConn
	ConnPolicy        proxyproto.ConnPolicyFunc
	ValidateHeader    proxyproto.Validator
	ReadHeaderTimeout time.Duration
}

func (c *PacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	for {
		n, addr, err = c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		n, addr, err = c.readFrom(p[:n], addr)
		if err != nil {
			// drop invalid packet as returning error would cause the ReadFrom caller to exit
			// which could result in DoS if an attacker sends intentional invalid packets
			clog.Warningf("dropping invalid Proxy Protocol packet from %s: %v", addr.String(), err)
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
			srcAddr, _, _ := header.UDPAddrs()
			addr = &Addr{u: addr, r: srcAddr}
		}
	default:
	}
	copy(p, payload)
	return len(payload), addr, nil
}

type Addr struct {
	u net.Addr
	r net.Addr
}

func (a *Addr) Network() string {
	return a.u.Network()
}

func (a *Addr) String() string {
	return a.r.String()
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
