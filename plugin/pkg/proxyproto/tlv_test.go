package proxyproto

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

// TestV1HeaderNoTLVs verifies PROXY protocol v1 (which has no TLVs) is handled
// cleanly: capture is a no-op, with no error or panic.
func TestV1HeaderNoTLVs(t *testing.T) {
	h := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53},
	}
	if got := rawTLVs(h); got != nil {
		t.Fatalf("rawTLVs(v1) = %v, want nil", got)
	}
	pc := &PacketConn{CaptureTLVs: true}
	if got := pc.tlvsFor(h); got != nil {
		t.Fatalf("tlvsFor(v1) = %v, want nil", got)
	}
	if tlvs, err := h.TLVs(); err != nil || len(tlvs) != 0 {
		t.Fatalf("v1 TLVs() = %v, %v; want empty, nil", tlvs, err)
	}
}

// TestAddrFromConnCommand covers both PROXY and LOCAL commands: PROXY publishes
// the TLVs, LOCAL must not (the PROXY protocol spec requires discarding the
// protocol block for LOCAL, even though go-proxyproto retains the parsed TLVs).
func TestAddrFromConnCommand(t *testing.T) {
	tlvs := []proxyproto.TLV{{Type: 0xEA, Value: []byte{0x01, 0x02}}}
	tests := []struct {
		name    string
		command proxyproto.ProtocolVersionAndCommand
		wantTLV bool
	}{
		{"PROXY", proxyproto.PROXY, true},
		{"LOCAL", proxyproto.LOCAL, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &proxyproto.Header{
				Version:           2,
				Command:           tc.command,
				TransportProtocol: proxyproto.TCPv4,
				SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000},
				DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53},
			}
			if err := h.SetTLVs(tlvs); err != nil {
				t.Fatalf("SetTLVs: %v", err)
			}
			raw, _ := h.Format()
			pc := proxyproto.NewConn(&preloadedConn{buf: raw})
			defer pc.Close()

			a := AddrFromConn(pc)
			if tc.wantTLV {
				if a == nil || len(a.RawTLVs()) == 0 {
					t.Fatalf("AddrFromConn(%s) did not return TLVs", tc.name)
				}
			} else if a != nil {
				t.Fatalf("AddrFromConn(%s) = %v, want nil (TLVs must not be published)", tc.name, a)
			}
		})
	}
}

// fakeListener returns a preset sequence of conns from Accept.
type fakeListener struct {
	conns  []net.Conn
	i      int
	addr   net.Addr
	closed bool
}

func (l *fakeListener) Accept() (net.Conn, error) {
	if l.i >= len(l.conns) {
		return nil, net.ErrClosed
	}
	c := l.conns[l.i]
	l.i++
	return c, nil
}
func (l *fakeListener) Close() error { l.closed = true; return nil }
func (l *fakeListener) Addr() net.Addr {
	if l.addr != nil {
		return l.addr
	}
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func TestNewTLVListenerAcceptWrapsProxyConn(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	h := &proxyproto.Header{
		Version: 2, Command: proxyproto.PROXY, TransportProtocol: proxyproto.TCPv4,
		SourceAddr: src, DestinationAddr: dst,
	}
	if err := h.SetTLVs([]proxyproto.TLV{{Type: 0xEA, Value: []byte{0x01, 0x02}}}); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw, _ := h.Format()
	pc := proxyproto.NewConn(&preloadedConn{buf: raw})

	// Inject a fake inner listener that hands back the *proxyproto.Conn, so we
	// test TLVListener.Accept's wrapping without driving the real proxyproto
	// Listener's own header/policy handling.
	l := &TLVListener{inner: &fakeListener{conns: []net.Conn{pc}}}
	defer l.Close()
	_ = l.Addr()

	c, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	a, ok := c.RemoteAddr().(*Addr)
	if !ok {
		t.Fatalf("Accept conn RemoteAddr type = %T, want *Addr", c.RemoteAddr())
	}
	got, err := proxyproto.SplitTLVs(a.RawTLVs())
	if err != nil || len(got) != 1 || !bytes.Equal(got[0].Value, []byte{0x01, 0x02}) {
		t.Fatalf("TLV mismatch: %+v err=%v", got, err)
	}
	c.Close()
}

func TestTLVListenerAcceptPassesThroughNonProxyConn(t *testing.T) {
	// A non-*proxyproto.Conn is returned unchanged.
	plain := plainRemoteConn{src: &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 7000}}
	l := &TLVListener{inner: &fakeListener{conns: []net.Conn{plain}}}
	c, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if _, ok := c.RemoteAddr().(*Addr); ok {
		t.Fatal("non-proxy conn should pass through without *Addr")
	}
}

func TestNewTLVListenerConstructor(t *testing.T) {
	// NewTLVListener returns a *TLVListener wrapping a proxyproto.Listener.
	base := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9953}}
	l := NewTLVListener(base, nil)
	if l.Addr().String() != base.Addr().String() {
		t.Fatalf("Addr = %s, want %s", l.Addr(), base.Addr())
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !base.closed {
		t.Fatal("expected inner listener to be closed")
	}
}

func TestNewAddr(t *testing.T) {
	u := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	r := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	a := NewAddr(u, r, []byte{0xaa})
	if a.Network() != u.Network() {
		t.Errorf("Network = %q, want %q", a.Network(), u.Network())
	}
	if a.String() != r.String() {
		t.Errorf("String = %q, want %q", a.String(), r.String())
	}
	if !bytes.Equal(a.RawTLVs(), []byte{0xaa}) {
		t.Errorf("RawTLVs = %v, want [aa]", a.RawTLVs())
	}
}

// tlsLikeConn wraps a net.Conn and exposes NetConn, mimicking *tls.Conn so the
// netConner branch of underlyingProxyConn is exercised.
type tlsLikeConn struct {
	net.Conn
	inner net.Conn
}

func (c *tlsLikeConn) NetConn() net.Conn { return c.inner }

func TestAddrFromConnUnwrapsTLSLayer(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	h := &proxyproto.Header{
		Version: 2, Command: proxyproto.PROXY, TransportProtocol: proxyproto.TCPv4,
		SourceAddr: src, DestinationAddr: dst,
	}
	if err := h.SetTLVs([]proxyproto.TLV{{Type: 0xEA, Value: []byte{0x07}}}); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw, _ := h.Format()
	pc := proxyproto.NewConn(&preloadedConn{buf: raw})

	// tlsLike -> *proxyproto.Conn. RemoteAddr on the plain tls-like wrapper does
	// NOT promote the proxy header, so AddrFromConn must fall through to the
	// NetConn-unwrap path to reach the *proxyproto.Conn.
	wrapped := &tlsLikeConn{Conn: plainRemoteConn{src: dst}, inner: pc}

	a := AddrFromConn(wrapped)
	if a == nil {
		t.Fatal("AddrFromConn did not unwrap tls-like layer")
	}
	got, err := proxyproto.SplitTLVs(a.RawTLVs())
	if err != nil || len(got) != 1 || !bytes.Equal(got[0].Value, []byte{0x07}) {
		t.Fatalf("TLV mismatch after unwrap: %+v err=%v", got, err)
	}
	pc.Close()
}

func TestAddrFromConnUnknownConn(t *testing.T) {
	// A conn that is neither *proxyproto.Conn, *tlvConn, nor a netConner, and
	// whose RemoteAddr is a plain addr: AddrFromConn returns nil.
	if a := AddrFromConn(plainRemoteConn{src: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5}}); a != nil {
		t.Fatalf("AddrFromConn(unknown) = %v, want nil", a)
	}
}

// plainRemoteConn is a net.Conn whose RemoteAddr is a plain *net.TCPAddr.
type plainRemoteConn struct {
	net.Conn
	src net.Addr
}

func (c plainRemoteConn) RemoteAddr() net.Addr { return c.src }

// preloadedConn is a net.Conn whose Read drains a fixed byte buffer (the PPv2
// header), avoiding net.Pipe synchronization in unit tests.
type preloadedConn struct {
	net.Conn
	buf    []byte
	off    int
	remote net.Addr
}

func (c *preloadedConn) Read(p []byte) (int, error) {
	if c.off >= len(c.buf) {
		return 0, io.EOF
	}
	n := copy(p, c.buf[c.off:])
	c.off += n
	return n, nil
}
func (c *preloadedConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *preloadedConn) Close() error                { return nil }
func (c *preloadedConn) RemoteAddr() net.Addr {
	if c.remote != nil {
		return c.remote
	}
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}
}
func (c *preloadedConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}
func (c *preloadedConn) SetDeadline(time.Time) error      { return nil }
func (c *preloadedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *preloadedConn) SetWriteDeadline(time.Time) error { return nil }

// ppv2UDPPacketWithTLVs builds a valid PPv2 UDP datagram carrying the given
// TLVs followed by the DNS payload, using the library's own formatter.
func ppv2UDPPacketWithTLVs(t *testing.T, src, dst *net.UDPAddr, tlvs []proxyproto.TLV, payload []byte) []byte {
	t.Helper()
	h := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.UDPv4,
		SourceAddr:        src,
		DestinationAddr:   dst,
	}
	if len(tlvs) > 0 {
		if err := h.SetTLVs(tlvs); err != nil {
			t.Fatalf("SetTLVs: %v", err)
		}
	}
	raw, err := h.Format()
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	return append(raw, payload...)
}

func usePolicy(proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
	return proxyproto.USE, nil
}

func TestReadFromCapturesTLVsWhenEnabled(t *testing.T) {
	src := udpAddr("192.168.1.10", 40000)
	dst := udpAddr("10.0.0.1", 53)
	payload := []byte("dns-payload")
	tlvs := []proxyproto.TLV{
		{Type: proxyproto.PP2_TYPE_AUTHORITY, Value: []byte("example.org")},
		{Type: 0xEA, Value: []byte{0x01, 0x02, 0x03}},
	}
	packet := ppv2UDPPacketWithTLVs(t, src, dst, tlvs, payload)

	pc := &PacketConn{PacketConn: &singlePacketConn{}, ConnPolicy: usePolicy, CaptureTLVs: true}
	n, addr, err := pc.readFrom(packet, udpAddr("203.0.113.9", 5000))
	if err != nil {
		t.Fatalf("readFrom: %v", err)
	}
	if !bytes.Equal(packet[:n], payload) {
		t.Fatalf("payload = %q, want %q", packet[:n], payload)
	}
	a, ok := addr.(*Addr)
	if !ok {
		t.Fatalf("addr type = %T, want *Addr", addr)
	}
	raw := a.RawTLVs()
	if len(raw) == 0 {
		t.Fatal("expected raw TLVs to be captured")
	}
	// Round-trip: the captured bytes must re-split to the TLVs we sent.
	got, err := proxyproto.SplitTLVs(raw)
	if err != nil {
		t.Fatalf("SplitTLVs: %v", err)
	}
	if len(got) != len(tlvs) {
		t.Fatalf("got %d TLVs, want %d", len(got), len(tlvs))
	}
	for i, want := range tlvs {
		if got[i].Type != want.Type || !bytes.Equal(got[i].Value, want.Value) {
			t.Errorf("TLV %d = {%#x,%v}, want {%#x,%v}", i, got[i].Type, got[i].Value, want.Type, want.Value)
		}
	}
}

func TestReadFromSkipsTLVsWhenDisabled(t *testing.T) {
	src := udpAddr("192.168.1.10", 40000)
	dst := udpAddr("10.0.0.1", 53)
	tlvs := []proxyproto.TLV{{Type: 0xEA, Value: []byte{0x09}}}
	packet := ppv2UDPPacketWithTLVs(t, src, dst, tlvs, []byte("x"))

	pc := &PacketConn{PacketConn: &singlePacketConn{}, ConnPolicy: usePolicy} // CaptureTLVs defaults to false
	_, addr, err := pc.readFrom(packet, udpAddr("203.0.113.9", 5000))
	if err != nil {
		t.Fatalf("readFrom: %v", err)
	}
	a, ok := addr.(*Addr)
	if !ok {
		t.Fatalf("addr type = %T, want *Addr", addr)
	}
	if a.RawTLVs() != nil {
		t.Fatalf("expected no TLVs captured when disabled, got %v", a.RawTLVs())
	}
}

func TestReadFromNoTLVsPresent(t *testing.T) {
	src := udpAddr("192.168.1.10", 40000)
	dst := udpAddr("10.0.0.1", 53)
	packet := ppv2UDPPacketWithTLVs(t, src, dst, nil, []byte("x"))

	pc := &PacketConn{PacketConn: &singlePacketConn{}, ConnPolicy: usePolicy, CaptureTLVs: true}
	_, addr, err := pc.readFrom(packet, udpAddr("203.0.113.9", 5000))
	if err != nil {
		t.Fatalf("readFrom: %v", err)
	}
	a, ok := addr.(*Addr)
	if !ok {
		t.Fatalf("addr type = %T, want *Addr", addr)
	}
	if a.RawTLVs() != nil {
		t.Fatalf("expected nil TLVs when header has none, got %v", a.RawTLVs())
	}
}

func TestRawTLVsHelper(t *testing.T) {
	// nil header -> nil
	if got := rawTLVs(nil); got != nil {
		t.Fatalf("rawTLVs(nil) = %v, want nil", got)
	}
	// header with no TLVs -> nil
	h := &proxyproto.Header{Version: 2}
	if got := rawTLVs(h); got != nil {
		t.Fatalf("rawTLVs(no tlvs) = %v, want nil", got)
	}
	// header with TLVs -> re-splittable bytes
	tlvs := []proxyproto.TLV{{Type: proxyproto.PP2_TYPE_UNIQUE_ID, Value: []byte("abc")}}
	if err := h.SetTLVs(tlvs); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw := rawTLVs(h)
	got, err := proxyproto.SplitTLVs(raw)
	if err != nil {
		t.Fatalf("SplitTLVs: %v", err)
	}
	if len(got) != 1 || got[0].Type != proxyproto.PP2_TYPE_UNIQUE_ID || !bytes.Equal(got[0].Value, []byte("abc")) {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestTLVConnRemoteAddrCarriesTLVs(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	tlvs := []proxyproto.TLV{{Type: 0xEA, Value: []byte{0xaa, 0xbb}}}
	h := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        src,
		DestinationAddr:   dst,
	}
	if err := h.SetTLVs(tlvs); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw, err := h.Format()
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	client, server := net.Pipe()
	go func() {
		client.Write(raw)
		// Keep the pipe open; ProxyHeader only needs the header bytes.
	}()

	pc := proxyproto.NewConn(server)
	tc := &tlvConn{Conn: pc, pc: pc}

	addr := tc.RemoteAddr()
	a, ok := addr.(*Addr)
	if !ok {
		t.Fatalf("RemoteAddr type = %T, want *Addr", addr)
	}
	if a.String() != src.String() {
		t.Errorf("RemoteAddr = %s, want %s", a.String(), src)
	}
	got, err := proxyproto.SplitTLVs(a.RawTLVs())
	if err != nil {
		t.Fatalf("SplitTLVs: %v", err)
	}
	if len(got) != 1 || got[0].Type != 0xEA || !bytes.Equal(got[0].Value, []byte{0xaa, 0xbb}) {
		t.Fatalf("TLV round-trip mismatch: %+v", got)
	}
	tc.Close()
}

func TestTLVConnRemoteAddrNoHeader(t *testing.T) {
	// A connection with no PROXY header: RemoteAddr falls back to the plain
	// address, not an *Addr.
	client, server := net.Pipe()
	go func() {
		client.Write([]byte("not a proxy header"))
	}()
	pc := proxyproto.NewConn(server)
	tc := &tlvConn{Conn: pc, pc: pc}
	if _, ok := tc.RemoteAddr().(*Addr); ok {
		t.Fatal("expected plain addr when no PROXY header present")
	}
	tc.Close()
}

// delegatingConn embeds a net.Conn and promotes its RemoteAddr, mimicking the
// wrapper chain (tls.Conn, connection limiter) the DoH path sees.
type delegatingConn struct {
	net.Conn
}

func TestAddrFromConnDirect(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	h := &proxyproto.Header{
		Version: 2, Command: proxyproto.PROXY, TransportProtocol: proxyproto.TCPv4,
		SourceAddr: src, DestinationAddr: dst,
	}
	if err := h.SetTLVs([]proxyproto.TLV{{Type: 0xEA, Value: []byte{0x01}}}); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw, _ := h.Format()
	client, server := net.Pipe()
	go func() { client.Write(raw) }()
	pc := proxyproto.NewConn(server)

	// Direct *proxyproto.Conn.
	if a := AddrFromConn(pc); a == nil || len(a.RawTLVs()) == 0 {
		t.Fatal("AddrFromConn(*proxyproto.Conn) did not recover TLVs")
	}
	pc.Close()
}

func TestAddrFromConnThroughWrapper(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 40000}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 53}
	h := &proxyproto.Header{
		Version: 2, Command: proxyproto.PROXY, TransportProtocol: proxyproto.TCPv4,
		SourceAddr: src, DestinationAddr: dst,
	}
	if err := h.SetTLVs([]proxyproto.TLV{{Type: 0xEA, Value: []byte{0x02, 0x03}}}); err != nil {
		t.Fatalf("SetTLVs: %v", err)
	}
	raw, _ := h.Format()
	client, server := net.Pipe()
	go func() { client.Write(raw) }()
	pc := proxyproto.NewConn(server)
	tc := &tlvConn{Conn: pc, pc: pc}
	// Wrap in a conn that only promotes RemoteAddr (like tls.Conn / limiter).
	wrapped := &delegatingConn{Conn: tc}

	a := AddrFromConn(wrapped)
	if a == nil {
		t.Fatal("AddrFromConn through wrapper returned nil")
	}
	got, err := proxyproto.SplitTLVs(a.RawTLVs())
	if err != nil || len(got) != 1 || !bytes.Equal(got[0].Value, []byte{0x02, 0x03}) {
		t.Fatalf("TLV mismatch through wrapper: %+v err=%v", got, err)
	}
	tc.Close()
}

func TestAddrFromConnNil(t *testing.T) {
	if a := AddrFromConn(nil); a != nil {
		t.Fatalf("AddrFromConn(nil) = %v, want nil", a)
	}
}
