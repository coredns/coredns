package dnsserver

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const (
	// DoQCodeInternalError signals that the DoQ implementation encountered
	// an internal error and is incapable of pursuing the transaction or the
	// connection.
	DoQCodeInternalError quic.ApplicationErrorCode = 1

	// DoQCodeProtocolError signals that the DoQ implementation encountered
	// a protocol error and is forcibly aborting the connection.
	DoQCodeProtocolError quic.ApplicationErrorCode = 2
)

// ServerQUIC represents an instance of a DNS-over-QUIC server.
type ServerQUIC struct {
	*Server
	listenAddr   net.Addr
	tlsConfig    *tls.Config
	quicConfig   *quic.Config
	quicListener *quic.Listener
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
		tlsConfig.NextProtos = []string{"doq"}
	}

	var quicConfig *quic.Config
	quicConfig = &quic.Config{
		MaxIdleTimeout:        s.idleTimeout,
		MaxIncomingStreams:    math.MaxUint16,
		MaxIncomingUniStreams: math.MaxUint16,
		// Enable 0-RTT by default for all connections on the server-side.
		Allow0RTT: true,
	}

	return &ServerQUIC{Server: s, tlsConfig: tlsConfig, quicConfig: quicConfig}, nil
}

// ServePacket implements caddy.UDPServer interface.
func (s *ServerQUIC) ServePacket(p net.PacketConn) error {
	s.m.Lock()
	s.listenAddr = s.quicListener.Addr()
	s.m.Unlock()

	return s.ServeQUIC()
}

// ServeQUIC listens for incoming QUIC packets.
func (s *ServerQUIC) ServeQUIC() error {
	for {
		conn, err := s.quicListener.Accept(context.Background())
		if err != nil {
			closeQUICConn(conn, DoQCodeInternalError)
			return err
		}

		go s.serveQUICConnection(conn)
	}
}

// serveQUICConnection handles a new QUIC connection. It waits for new streams
// and passes them to serveQUICStream.
func (s *ServerQUIC) serveQUICConnection(conn quic.Connection) {
	for {
		// In DoQ, one query consumes one stream.
		// The client MUST select the next available client-initiated bidirectional
		// stream for each subsequent query on a QUIC connection.
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			closeQUICConn(conn, DoQCodeInternalError)
			return
		}

		go s.serveQUICStream(stream, conn)
	}
}

func (s *ServerQUIC) serveQUICStream(stream quic.Stream, conn quic.Connection) {
	buf, err := readDOQMessage(stream)

	// io.EOF does not really mean that there's any error, it is just
	// the STREAM FIN indicating that there will be no data to read
	// anymore from this stream.
	if err != nil && err != io.EOF {
		closeQUICConn(conn, DoQCodeProtocolError)

		return
	}

	req := &dns.Msg{}
	err = req.Unpack(buf)
	if err != nil {
		clog.Debugf("unpacking quic packet: %s", err)
		closeQUICConn(conn, DoQCodeProtocolError)

		return
	}

	if !validRequest(req) {
		// If a peer encounters such an error condition, it is considered a
		// fatal error. It SHOULD forcibly abort the connection using QUIC's
		// CONNECTION_CLOSE mechanism and SHOULD use the DoQ error code
		// DOQ_PROTOCOL_ERROR.
		// See https://www.rfc-editor.org/rfc/rfc9250#section-4.3.3-3
		closeQUICConn(conn, DoQCodeProtocolError)

		return
	}

	w := &DoQWriter{
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
		stream:     stream,
		Msg:        req,
	}

	dnsCtx := context.WithValue(stream.Context(), Key{}, s.Server)
	dnsCtx = context.WithValue(dnsCtx, LoopKey{}, 0)
	s.ServeDNS(dnsCtx, w, req)
}

// ListenPacket implements caddy.UDPServer interface.
func (s *ServerQUIC) ListenPacket() (net.PacketConn, error) {
	p, err := reuseport.ListenPacket("udp", s.Addr[len(transport.QUIC+"://"):])
	if err != nil {
		return nil, err
	}

	s.m.Lock()
	defer s.m.Unlock()

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

// Stop stops the server non-gracefully. It blocks until the server is totally stopped.
func (s *ServerQUIC) Stop() error {
	s.m.Lock()
	defer s.m.Unlock()

	if s.quicListener != nil {
		return s.quicListener.Close()
	}

	return nil
}

// Serve implements caddy.TCPServer interface.
func (s *ServerQUIC) Serve(l net.Listener) error { return nil }

// Listen implements caddy.TCPServer interface.
func (s *ServerQUIC) Listen() (net.Listener, error) { return nil, nil }

// closeQUICConn quietly closes the QUIC connection.
func closeQUICConn(conn quic.Connection, code quic.ApplicationErrorCode) {
	if conn == nil {
		return
	}

	clog.Debugf("closing quic conn %s with code %d", conn.LocalAddr(), code)

	err := conn.CloseWithError(code, "")
	if err != nil {
		clog.Debugf("closing quic connection with code %d: %s", code, err)
	}
}

// validRequest checks for protocol errors in the unpacked DNS message.
// See https://www.rfc-editor.org/rfc/rfc9250.html#name-protocol-errors
func validRequest(req *dns.Msg) (ok bool) {
	// 1. a client or server receives a message with a non-zero Message ID.
	if req.Id != 0 {
		return false
	}

	// 2. an implementation receives a message containing the edns-tcp-keepalive
	// EDNS(0) Option [RFC7828].
	if opt := req.IsEdns0(); opt != nil {
		for _, option := range opt.Option {
			if option.Option() == dns.EDNS0TCPKEEPALIVE {
				clog.Debug("client sent EDNS0 TCP keepalive option")

				return false
			}
		}
	}

	// 3. the client or server does not indicate the expected STREAM FIN after
	// sending requests or responses.
	//
	// This is quite problematic to validate this case since this would imply
	// we have to wait until STREAM FIN is arrived before we start processing
	// the message. So we're consciously ignoring this case in this
	// implementation.

	// 4. a server receives a "replayable" transaction in 0-RTT data
	//
	// The information necessary to validate this is not exposed by quic-go.

	return true
}

// readDOQMessage reads a DNS over QUIC (DOQ) message from the given stream
// and returns the message bytes.
// Drafts of the RFC9250 did not require the 2-byte prefixed message length.
// Thus, we are only supporting the official version (DoQ v1).
func readDOQMessage(r io.Reader) ([]byte, error) {
	// All DNS messages (queries and responses) sent over DoQ connections MUST
	// be encoded as a 2-octet length field followed by the message content as
	// specified in [RFC1035].
	// See https://www.rfc-editor.org/rfc/rfc9250.html#section-4.2-4
	sizeBuf := make([]byte, 2)
	_, err := io.ReadFull(r, sizeBuf)
	if err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint16(sizeBuf)
	buf := make([]byte, size)
	_, err = io.ReadFull(r, buf)

	// A client or server receives a STREAM FIN before receiving all the bytes
	// for a message indicated in the 2-octet length field.
	// See https://www.rfc-editor.org/rfc/rfc9250#section-4.3.3-2.2
	if size != uint16(len(buf)) {
		return nil, fmt.Errorf("message size does not match 2-byte prefix")
	}

	return buf, err
}
