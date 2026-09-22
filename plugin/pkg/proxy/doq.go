package proxy

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const (
	doqALPN               = "doq"
	doqDialTimeout        = 5 * time.Second
	doqDefaultIdleTimeout = 30 * time.Second
	doqProtocolError      = quic.ApplicationErrorCode(0x2)
	doqRequestCancelled   = quic.StreamErrorCode(0x3)
)

var errDoQProtocol = errors.New("DNS-over-QUIC protocol error")

type doqConn struct {
	conn      *quic.Conn
	transport *quic.Transport
	created   time.Time
	lastUsed  time.Time
	active    int
	draining  bool
	closed    bool
}

// doqTransport owns one reusable QUIC connection. Queries share the
// connection, but each query uses its own bidirectional stream as required by
// RFC 9250.
type doqTransport struct {
	proxyName string
	addr      string

	mu              sync.Mutex
	tlsConfig       *tls.Config
	localAddress    net.IP
	expire          time.Duration
	maxAge          time.Duration
	readTimeout     time.Duration
	current         *doqConn
	connections     map[*doqConn]struct{}
	dialDone        chan struct{}
	started         bool
	stopped         bool
	stop            chan struct{}
	stopOnce        sync.Once
	lifecycleCtx    context.Context
	cancelLifecycle context.CancelFunc
}

func newDoQTransport(proxyName, addr string) *doqTransport {
	lifecycleCtx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- stopTransport calls the stored cancel function
	return &doqTransport{
		proxyName:       proxyName,
		addr:            addr,
		expire:          defaultExpire,
		readTimeout:     maxTimeout,
		connections:     make(map[*doqConn]struct{}),
		stop:            make(chan struct{}),
		lifecycleCtx:    lifecycleCtx,
		cancelLifecycle: cancel,
	}
}

func (t *doqTransport) setTLSConfig(cfg *tls.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cfg == nil {
		t.tlsConfig = nil
		return
	}
	t.tlsConfig = cfg.Clone()
}

func (t *doqTransport) setLocalAddress(addr net.IP) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.localAddress = append(net.IP(nil), addr...)
}

func (t *doqTransport) setExpire(expire time.Duration) {
	t.mu.Lock()
	t.expire = expire
	t.mu.Unlock()
}

func (t *doqTransport) setMaxAge(maxAge time.Duration) {
	t.mu.Lock()
	t.maxAge = maxAge
	t.mu.Unlock()
}

func (t *doqTransport) setReadTimeout(timeout time.Duration) {
	t.mu.Lock()
	t.readTimeout = timeout
	t.mu.Unlock()
}

func (t *doqTransport) start() {
	t.mu.Lock()
	if t.started || t.stopped {
		t.mu.Unlock()
		return
	}
	t.started = true
	t.mu.Unlock()

	go t.connManager()
}

func (t *doqTransport) connManager() {
	ticker := time.NewTicker(defaultExpire)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			t.cleanup(now)
		case <-t.stop:
			return
		}
	}
}

func (t *doqTransport) stopTransport() {
	t.stopOnce.Do(func() {
		t.cancelLifecycle()
		t.mu.Lock()
		t.stopped = true
		close(t.stop)
		connections := make([]*doqConn, 0, len(t.connections))
		for c := range t.connections {
			if c.closed {
				continue
			}
			c.closed = true
			connections = append(connections, c)
		}
		t.current = nil
		clear(t.connections)
		t.mu.Unlock()

		for _, c := range connections {
			closeDoQConn(c, 0, "")
		}
	})
}

func (t *doqTransport) cleanup(now time.Time) {
	var toClose []*doqConn

	t.mu.Lock()
	if c := t.current; c != nil {
		dead := c.conn.Context().Err() != nil
		expired := c.active == 0 && (t.expire == 0 || now.Sub(c.lastUsed) >= t.expire)
		tooOld := t.maxAge > 0 && now.Sub(c.created) >= t.maxAge
		if dead || expired || tooOld {
			t.current = nil
			c.draining = true
		}
	}
	for c := range t.connections {
		if c.draining && c.active == 0 && !c.closed {
			c.closed = true
			delete(t.connections, c)
			toClose = append(toClose, c)
		}
	}
	t.mu.Unlock()

	for _, c := range toClose {
		closeDoQConn(c, 0, "")
	}
}

func (t *doqTransport) acquire(ctx context.Context) (*doqConn, bool, error) {
	for {
		t.cleanup(time.Now())

		t.mu.Lock()
		if t.stopped {
			t.mu.Unlock()
			return nil, false, errors.New(ErrTransportStopped)
		}
		if c := t.current; c != nil {
			c.active++
			t.mu.Unlock()
			connCacheHitsCount.WithLabelValues(t.proxyName, t.addr, transport.QUIC).Inc()
			return c, true, nil
		}
		if done := t.dialDone; done != nil {
			t.mu.Unlock()
			select {
			case <-done:
				continue
			case <-t.stop:
				return nil, false, errors.New(ErrTransportStopped)
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}

		done := make(chan struct{})
		t.dialDone = done
		tlsConfig := cloneDoQTLSConfig(t.tlsConfig)
		localAddress := append(net.IP(nil), t.localAddress...)
		idleTimeout := max(doqDefaultIdleTimeout, t.expire, t.readTimeout)
		t.mu.Unlock()

		connCacheMissesCount.WithLabelValues(t.proxyName, t.addr, transport.QUIC).Inc()
		dialCtx, cancelDial := context.WithCancel(ctx)
		stopDial := context.AfterFunc(t.lifecycleCtx, cancelDial)
		c, err := dialDoQ(dialCtx, t.addr, localAddress, tlsConfig, idleTimeout)
		stopDial()
		cancelDial()

		t.mu.Lock()
		t.dialDone = nil
		close(done)
		if err == nil && !t.stopped {
			c.active = 1
			t.current = c
			t.connections[c] = struct{}{}
			t.mu.Unlock()
			return c, false, nil
		}
		stopped := t.stopped
		t.mu.Unlock()

		if c != nil {
			closeDoQConn(c, 0, "")
		}
		if stopped {
			return nil, false, errors.New(ErrTransportStopped)
		}
		return nil, false, err
	}
}

func cloneDoQTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		cfg = new(tls.Config)
	} else {
		cfg = cfg.Clone()
	}
	// DoQ uses a dedicated ALPN. Do not offer another application protocol on
	// this connection.
	cfg.NextProtos = []string{doqALPN}
	return cfg
}

func dialDoQ(ctx context.Context, addr string, localAddress net.IP, tlsConfig *tls.Config, idleTimeout time.Duration) (*doqConn, error) {
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	network := "udp6"
	if remote.IP.To4() != nil {
		network = "udp4"
	}
	local := &net.UDPAddr{IP: localAddress}
	packetConn, err := net.ListenUDP(network, local)
	if err != nil {
		return nil, err
	}

	quicTransport := &quic.Transport{Conn: packetConn}
	quicConfig := &quic.Config{
		HandshakeIdleTimeout:  doqDialTimeout,
		MaxIncomingStreams:    -1,
		MaxIncomingUniStreams: -1,
	}
	quicConfig.MaxIdleTimeout = idleTimeout

	dialCtx, cancel := context.WithTimeout(ctx, doqDialTimeout)
	defer cancel()
	conn, err := quicTransport.Dial(dialCtx, remote, tlsConfig, quicConfig)
	if err != nil {
		_ = quicTransport.Close()
		return nil, err
	}

	now := time.Now()
	return &doqConn{
		conn:      conn,
		transport: quicTransport,
		created:   now,
		lastUsed:  now,
	}, nil
}

func (t *doqTransport) release(c *doqConn) {
	var closeConn bool

	t.mu.Lock()
	if c.active > 0 {
		c.active--
	}
	c.lastUsed = time.Now()
	if c.draining && c.active == 0 && !c.closed {
		c.closed = true
		delete(t.connections, c)
		closeConn = true
	}
	t.mu.Unlock()

	if closeConn {
		closeDoQConn(c, 0, "")
	}
}

func (t *doqTransport) retire(c *doqConn, code quic.ApplicationErrorCode, reason string, abort bool) {
	var closeConn bool

	t.mu.Lock()
	if t.current == c {
		t.current = nil
	}
	c.draining = true
	if c.active == 0 && !c.closed {
		c.closed = true
		delete(t.connections, c)
		closeConn = true
	}
	t.mu.Unlock()

	if abort {
		_ = c.conn.CloseWithError(code, reason)
	}
	if closeConn {
		closeDoQConn(c, code, reason)
	}
}

func closeDoQConn(c *doqConn, code quic.ApplicationErrorCode, reason string) {
	if c == nil {
		return
	}
	if c.conn != nil {
		_ = c.conn.CloseWithError(code, reason)
	}
	if c.transport != nil {
		_ = c.transport.Close()
	}
}

func (t *doqTransport) exchange(ctx context.Context, msg *dns.Msg, timeout time.Duration) (*dns.Msg, net.Addr, error) {
	if isDNSZoneTransfer(msg) {
		return nil, nil, fmt.Errorf("%w: zone transfers over DoQ require multi-message response support", ErrUnsupportedRequest)
	}
	query := msg.Copy()
	query.Id = 0
	removeEDNSTCPKeepalive(query)
	wire, err := query.Pack()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	if len(wire) > int(^uint16(0)) {
		return nil, nil, fmt.Errorf("%w: DNS message is too large for DoQ", ErrInvalidRequest)
	}

	c, cached, err := t.acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	localAddr := c.conn.LocalAddr()
	if timeout <= 0 {
		t.mu.Lock()
		timeout = t.readTimeout
		t.mu.Unlock()
	}
	queryCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		queryCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	stream, err := c.conn.OpenStreamSync(queryCtx)
	if err != nil {
		if queryCtx.Err() != nil {
			t.release(c)
			return nil, localAddr, queryCtx.Err()
		}
		t.retire(c, 0, "", true)
		t.release(c)
		if cached {
			return nil, localAddr, ErrCachedClosed
		}
		return nil, localAddr, err
	}
	defer t.release(c)

	stopCancellation := context.AfterFunc(queryCtx, func() {
		stream.CancelRead(doqRequestCancelled)
		stream.CancelWrite(doqRequestCancelled)
	})
	defer stopCancellation()

	if err = writeDOQMessage(stream, wire); err != nil {
		stream.CancelRead(doqRequestCancelled)
		stream.CancelWrite(doqRequestCancelled)
		if queryCtx.Err() != nil {
			return nil, localAddr, queryCtx.Err()
		}
		if c.conn.Context().Err() != nil {
			t.retire(c, 0, "", true)
		}
		return nil, localAddr, err
	}
	if err = stream.Close(); err != nil {
		stream.CancelRead(doqRequestCancelled)
		if queryCtx.Err() != nil {
			return nil, localAddr, queryCtx.Err()
		}
		if c.conn.Context().Err() != nil {
			t.retire(c, 0, "", true)
		}
		return nil, localAddr, err
	}

	responseWire, err := readDOQMessage(stream)
	if err != nil {
		if errors.Is(err, errDoQProtocol) {
			t.retire(c, doqProtocolError, err.Error(), true)
		} else {
			stream.CancelRead(doqRequestCancelled)
			if c.conn.Context().Err() != nil {
				t.retire(c, 0, "", true)
			}
		}
		if queryCtx.Err() != nil {
			return nil, localAddr, queryCtx.Err()
		}
		return nil, localAddr, err
	}
	if err = expectDOQFIN(stream); err != nil {
		if errors.Is(err, errDoQProtocol) {
			t.retire(c, doqProtocolError, err.Error(), true)
		} else {
			stream.CancelRead(doqRequestCancelled)
		}
		if queryCtx.Err() != nil {
			return nil, localAddr, queryCtx.Err()
		}
		return nil, localAddr, err
	}

	response := new(dns.Msg)
	if err = response.Unpack(responseWire); err != nil {
		err = fmt.Errorf("%w: invalid DNS response: %v", errDoQProtocol, err)
		t.retire(c, doqProtocolError, err.Error(), true)
		return nil, localAddr, err
	}
	if response.Id != 0 {
		err = fmt.Errorf("%w: response message ID is %d, want 0", errDoQProtocol, response.Id)
		t.retire(c, doqProtocolError, err.Error(), true)
		return nil, localAddr, err
	}
	response.Id = msg.Id
	return response, localAddr, nil
}

func isDNSZoneTransfer(msg *dns.Msg) bool {
	return len(msg.Question) == 1 && (msg.Question[0].Qtype == dns.TypeAXFR || msg.Question[0].Qtype == dns.TypeIXFR)
}

func removeEDNSTCPKeepalive(msg *dns.Msg) {
	opt := msg.IsEdns0()
	if opt == nil {
		return
	}
	options := opt.Option[:0]
	for _, option := range opt.Option {
		if option.Option() != dns.EDNS0TCPKEEPALIVE {
			options = append(options, option)
		}
	}
	opt.Option = options
}

func writeDOQMessage(w io.Writer, msg []byte) error {
	frame := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(frame, uint16(len(msg))) // #nosec G115 -- checked by caller
	copy(frame[2:], msg)
	for len(frame) > 0 {
		n, err := w.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}

func readDOQMessage(r io.Reader) ([]byte, error) {
	var sizeBytes [2]byte
	if _, err := io.ReadFull(r, sizeBytes[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: incomplete message length", errDoQProtocol)
		}
		return nil, err
	}
	size := binary.BigEndian.Uint16(sizeBytes[:])
	if size == 0 {
		return nil, fmt.Errorf("%w: zero-length DNS message", errDoQProtocol)
	}
	msg := make([]byte, int(size))
	if _, err := io.ReadFull(r, msg); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: message ended before %d bytes", errDoQProtocol, size)
		}
		return nil, err
	}
	return msg, nil
}

func expectDOQFIN(r io.Reader) error {
	var extra [1]byte
	for {
		n, err := r.Read(extra[:])
		if n != 0 {
			return fmt.Errorf("%w: multiple responses on one query stream", errDoQProtocol)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
