package dnsserver

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
)

// DoQVersion is an enumeration with supported DoQ versions.
type DoQVersion int

const (
	// DoQv1Draft represents old DoQ draft versions that do not send a 2-octet
	// prefix with the DNS message length.
	DoQv1Draft DoQVersion = 0x00

	// DoQv1 represents DoQ v1.0: https://www.rfc-editor.org/rfc/rfc9250.html.
	DoQv1 DoQVersion = 0x01
)

// ServerQUIC represents an instance of a DNS-over-QUIC server.
type ServerQUIC struct {
	*Server
	listenAddr   net.Addr
	tlsConfig    *tls.Config
	quicConfig   *quic.Config
	quicListener *quic.Listener

	// bytesPool is a pool of byte slices used to read DNS packets.
	bytesPool *sync.Pool
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

	if tlsConfig != nil {
		tlsConfig.NextProtos = []string{"doq", "doq-i03"}
	}

	bytesPool := &sync.Pool{
		New: func() interface{} {
			// 2 bytes may be used to store packet length (see TCP/TLS)
			b := make([]byte, 2+dns.MaxMsgSize)
			return &b
		},
	}

	var quicConfig *quic.Config
	quicConfig = &quic.Config{
		MaxIdleTimeout:        s.idleTimeout,
		MaxIncomingStreams:    math.MaxUint16,
		MaxIncomingUniStreams: math.MaxUint16,
		// Disable 0-RTT by default for all connections on the server-side.
		Allow0RTT: false,
	}

	return &ServerQUIC{Server: s, tlsConfig: tlsConfig, quicConfig: quicConfig, bytesPool: bytesPool}, nil
}

// ServePacket implements caddy.UDPServer interface.
func (s *ServerQUIC) ServePacket(p net.PacketConn) error {
	s.m.Lock()
	s.listenAddr = s.quicListener.Addr()
	s.m.Unlock()

	return s.ServeQUIC()
}

func (s *ServerQUIC) ServeQUIC() error {
	for {
		conn, err := s.quicListener.Accept(context.Background())
		if err != nil {
			return err
		}

		go s.serveQUICConnection(conn)
	}
}

func (s *ServerQUIC) serveQUICConnection(conn quic.Connection) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			// close connection with error
			return
		}

		go func() {
			s.serveQUICStream(stream, conn)

			// The server MUST send the response(s) on the same stream and MUST
			// indicate, after the last response, through the STREAM FIN
			// mechanism that no further data will be sent on that stream.
			_ = stream.Close()
		}()

	}
}

func (s *ServerQUIC) serveQUICStream(stream quic.Stream, conn quic.Connection) {
	bufPtr := s.bytesPool.Get().(*[]byte)
	defer s.bytesPool.Put(bufPtr)
	buf := *bufPtr
	n, err := readAll(stream, buf)

	doqVersion := DoQv1
	req := &dns.Msg{}

	packetLen := binary.BigEndian.Uint16(buf[:2])
	if packetLen == uint16(n-2) {
		err = req.Unpack(buf[2:])
	} else {
		err = req.Unpack(buf)
		doqVersion = DoQv1Draft
	}

	if err != nil {
		return
	}

	w := &quicResponse{
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
		stream:     stream,
		doqVersion: doqVersion,
		Msg:        req,
	}

	dnsCtx := context.WithValue(context.Background(), Key{}, s.Server)
	dnsCtx = context.WithValue(dnsCtx, LoopKey{}, 0)
	s.ServeDNS(dnsCtx, w, req)
}

// AddPrefix adds a 2-byte prefix with the DNS message length.
func AddPrefix(b []byte) (m []byte) {
	m = make([]byte, 2+len(b))
	binary.BigEndian.PutUint16(m, uint16(len(b)))
	copy(m[2:], b)

	return m
}

// ListenPacket implements caddy.UDPServer interface.
func (s *ServerQUIC) ListenPacket() (net.PacketConn, error) {
	p, err := reuseport.ListenPacket("udp", s.Addr[len(transport.QUIC+"://"):])
	if err != nil {
		return nil, err
	}

	s.quicListener, err = quic.Listen(p, s.tlsConfig, s.quicConfig)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// OnStartupComplete lists the sites served by this server
// and any relevant information, assuming Quiet is false.
func (s *ServerQUIC) OnStartupComplete() {
	if Quiet {
		return
	}

	out := startUpZones(transport.QUIC+"://", s.Addr, s.zones)
	if out != "" {
		fmt.Print(out)
	}
}

// Stop stops the server. It blocks until the server is totally stopped.
func (s *ServerQUIC) Stop() error { return nil }

func (s *ServerQUIC) countResponse(status int) {
	vars.HTTPSResponsesCount.WithLabelValues(s.Addr, strconv.Itoa(status)).Inc()
}

// Shutdown stops the server (non gracefully).
func (s *ServerQUIC) Shutdown() error { return nil }

// Serve implements caddy.TCPServer interface.
func (s *ServerQUIC) Serve(l net.Listener) error { return nil }

// Listen implements caddy.TCPServer interface.
func (s *ServerQUIC) Listen() (net.Listener, error) { return nil, nil }

// readAll reads from r until an error or io.EOF into the specified buffer buf.
// A successful call returns err == nil, not err == io.EOF.  If the buffer is
// too small, it returns error io.ErrShortBuffer.  This function has some
// similarities to io.ReadAll, but it reads to the specified buffer and not
// allocates (and grows) a new one.  Also, it is completely different from
// io.ReadFull as that one reads the exact number of bytes (buffer length) and
// readAll reads until io.EOF or until the buffer is filled.
func readAll(r io.Reader, buf []byte) (n int, err error) {
	for {
		if n == len(buf) {
			return n, io.ErrShortBuffer
		}

		var read int
		read, err = r.Read(buf[n:])
		n += read

		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return n, err
		}
	}
}

type quicResponse struct {
	localAddr  net.Addr
	remoteAddr net.Addr
	stream     quic.Stream
	doqVersion DoQVersion
	Msg        *dns.Msg
}

func (r *quicResponse) Write(b []byte) (int, error) {
	var respBuf []byte
	switch r.doqVersion {
	case DoQv1:
		respBuf = AddPrefix(b)
	case DoQv1Draft:
		respBuf = b
	default:
		return 0, fmt.Errorf("invalid protocol version: %d", r.doqVersion)
	}

	return r.stream.Write(respBuf)
}

func (r *quicResponse) WriteMsg(m *dns.Msg) error {
	bytes, err := m.Pack()
	if err != nil {
		return err
	}

	_, err = r.Write(bytes)
	return err
}

// These methods implement the dns.ResponseWriter interface from Go DNS.
func (r *quicResponse) Close() error          { return nil }
func (r *quicResponse) TsigStatus() error     { return nil }
func (r *quicResponse) TsigTimersOnly(b bool) {}
func (r *quicResponse) Hijack()               {}
func (r *quicResponse) LocalAddr() net.Addr   { return r.localAddr }
func (r *quicResponse) RemoteAddr() net.Addr  { return r.remoteAddr }
