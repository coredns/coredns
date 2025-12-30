package dnsserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/doh"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	// DefaultHTTPS3MaxStreams is the default maximum number of concurrent QUIC streams per connection.
	DefaultHTTPS3MaxStreams = 256
)

// ServerHTTPS3 represents a DNS-over-HTTP/3 server.
type ServerHTTPS3 struct {
	*Server
	httpsServer  *http3.Server
	listenAddr   net.Addr
	tlsConfig    *tls.Config
	quicConfig   *quic.Config
	validRequest func(*http.Request) bool
	maxStreams   int
}

// NewServerHTTPS3 builds the HTTP/3 (DoH3) server.
func NewServerHTTPS3(addr string, group []*Config) (*ServerHTTPS3, error) {
	s, err := NewServer(addr, group)
	if err != nil {
		return nil, err
	}

	// Extract TLS config (CoreDNS guarantees it is consistent)
	var tlsConfig *tls.Config
	for _, z := range s.zones {
		for _, conf := range z {
			tlsConfig = conf.TLSConfig
		}
	}
	if tlsConfig == nil {
		return nil, fmt.Errorf("DoH3 requires TLS, no TLS config found")
	}

	// HTTP/3 requires ALPN "h3"
	tlsConfig.NextProtos = []string{"h3"}

	// Request validator
	var validator func(*http.Request) bool
	for _, z := range s.zones {
		for _, conf := range z {
			validator = conf.HTTPRequestValidateFunc
		}
	}
	if validator == nil {
		validator = func(r *http.Request) bool { return r.URL.Path == doh.Path }
	}

	maxStreams := DefaultHTTPS3MaxStreams
	if len(group) > 0 && group[0] != nil && group[0].MaxHTTPS3Streams != nil {
		maxStreams = *group[0].MaxHTTPS3Streams
	}

	// QUIC transport config with stream limits (0 means use QUIC default)
	qconf := &quic.Config{
		MaxIdleTimeout: s.IdleTimeout,
		Allow0RTT:      true,
	}
	if maxStreams > 0 {
		qconf.MaxIncomingStreams = int64(maxStreams)
		qconf.MaxIncomingUniStreams = int64(maxStreams)
	}

	h3srv := &http3.Server{
		Handler:         nil, // set after constructing ServerHTTPS3
		TLSConfig:       tlsConfig,
		EnableDatagrams: true,
		QUICConfig:      qconf,
		//Logger: stdlog.New(&loggerAdapter{}, "", 0), TODO: Fix it
	}

	sh := &ServerHTTPS3{
		Server:       s,
		tlsConfig:    tlsConfig,
		httpsServer:  h3srv,
		quicConfig:   qconf,
		validRequest: validator,
		maxStreams:   maxStreams,
	}

	h3srv.Handler = sh

	return sh, nil
}

var _ caddy.GracefulServer = &ServerHTTPS3{}

// ListenPacket opens the UDP socket for QUIC.
func (s *ServerHTTPS3) ListenPacket() (net.PacketConn, error) {
	return reuseport.ListenPacket("udp", s.Addr[len(transport.HTTPS3+"://"):])
}

// ServePacket starts serving QUIC+HTTP/3 on an existing UDP socket.
func (s *ServerHTTPS3) ServePacket(pc net.PacketConn) error {
	s.m.Lock()
	s.listenAddr = pc.LocalAddr()
	s.m.Unlock()
	// Serve HTTP/3 over QUIC
	return s.httpsServer.Serve(pc)
}

// Listen function not used in HTTP/3, but defined for compatibility
func (s *ServerHTTPS3) Listen() (net.Listener, error) { return nil, nil }
func (s *ServerHTTPS3) Serve(l net.Listener) error    { return nil }

// OnStartupComplete lists the sites served by this server
// and any relevant information, assuming Quiet is false.
func (s *ServerHTTPS3) OnStartupComplete() {
	if Quiet {
		return
	}
	out := startUpZones(transport.HTTPS3+"://", s.Addr, s.zones)
	if out != "" {
		fmt.Print(out)
	}
}

// Stop graceful shutdown. It blocks until the server is totally stopped.
func (s *ServerHTTPS3) Stop() error {
	s.m.Lock()
	defer s.m.Unlock()
	if s.httpsServer != nil {
		return s.httpsServer.Shutdown(context.Background())
	}
	return nil
}

// Shutdown stops the server (non gracefully).
func (s *ServerHTTPS3) Shutdown() error {
	if s.httpsServer != nil {
		s.httpsServer.Shutdown(context.Background())
	}
	return nil
}

// ServeHTTP is the handler for the DoH3 requests
func (s *ServerHTTPS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.validRequest(r) {
		http.Error(w, "", http.StatusNotFound)
		s.countResponse(http.StatusNotFound)
		return
	}

	msg, err := doh.RequestToMsg(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		s.countResponse(http.StatusBadRequest)
		return
	}

	// from HTTP request → DNS writer
	h, p, _ := net.SplitHostPort(r.RemoteAddr)
	port, _ := strconv.Atoi(p)
	dw := &DoHWriter{
		laddr:   s.listenAddr,
		raddr:   &net.UDPAddr{IP: net.ParseIP(h), Port: port},
		request: r,
	}

	ctx := context.WithValue(r.Context(), Key{}, s.Server)
	ctx = context.WithValue(ctx, LoopKey{}, 0)
	ctx = context.WithValue(ctx, HTTPRequestKey{}, r)

	s.ServeDNS(ctx, dw, msg)

	if dw.Msg == nil {
		http.Error(w, "No response", http.StatusInternalServerError)
		s.countResponse(http.StatusInternalServerError)
		return
	}

	buf, _ := dw.Msg.Pack()
	mt, _ := response.Typify(dw.Msg, time.Now().UTC())
	age := dnsutil.MinimalTTL(dw.Msg, mt)

	w.Header().Set("Content-Type", doh.MimeType)
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", uint32(age.Seconds())))
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	w.WriteHeader(http.StatusOK)

	s.countResponse(http.StatusOK)
	w.Write(buf)
}

func (s *ServerHTTPS3) countResponse(status int) {
	vars.HTTPS3ResponsesCount.WithLabelValues(s.Addr, strconv.Itoa(status)).Inc()
}
