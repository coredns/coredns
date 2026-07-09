package dnstap

import (
	"net"
	"testing"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
)

func TestListenerCreation(t *testing.T) {
	tests := []struct {
		proto    string
		endpoint string
	}{
		{"tcp", "127.0.0.1:16000"},
		{"unix", "/tmp/dnstap-test.sock"},
	}

	for _, tc := range tests {
		l := newListener(tc.proto, tc.endpoint)
		if l.proto != tc.proto {
			t.Errorf("Expected proto %s, got %s", tc.proto, l.proto)
		}
		if l.endpoint != tc.endpoint {
			t.Errorf("Expected endpoint %s, got %s", tc.endpoint, l.endpoint)
		}
		if l.skipVerify != false {
			t.Errorf("Expected skipVerify to be false by default")
		}
		if len(l.clients) != 0 {
			t.Errorf("Expected clients map to be empty")
		}
	}
}

func TestListenerBroadcast(_ *testing.T) {
	l := newListener("tcp", "127.0.0.1:16001")

	// Verify that calling Dnstap with no clients doesn't panic
	msgType := tap.Dnstap_MESSAGE
	testPayload := &tap.Dnstap{
		Type: &msgType,
		Message: &tap.Message{
			QueryAddress: net.ParseIP("10.0.0.1").To4(),
		},
	}

	// Should not panic with no clients
	l.Dnstap(testPayload)

	// Add a mock client (without encoder to avoid framestream handshake complexity)
	mockConn := &mockConn{writes: [][]byte{}}
	c := &client{
		conn: mockConn,
		enc:  nil, // Set to nil to avoid framestream issues in test
		quit: make(chan struct{}),
	}

	l.clients[c] = struct{}{}

	// Broadcast should handle nil encoder gracefully (will call removeClient on error)
	l.Dnstap(testPayload)
}

// TestListenerDnstapFlushErrorNoDeadlock is a regression test for a self-deadlock:
// Dnstap held clientsMu.RLock and, when a client's flush() failed, called
// removeClient inline. removeClient takes clientsMu.Lock, which a non-reentrant
// RWMutex can never grant to a goroutine already holding the RLock, wedging every
// future broadcast and close(). A flush failure is the normal case for a slow or
// disconnected sink client, so the whole listen path must survive it.
func TestListenerDnstapFlushErrorNoDeadlock(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Peer completes the framestream bidirectional handshake so newEncoder succeeds.
	stop := make(chan struct{})
	defer close(stop)
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := fs.NewDecoder(conn, &fs.DecoderOptions{
			ContentType:   []byte("protobuf:dnstap.Dnstap"),
			Bidirectional: true,
		}); err != nil {
			return
		}
		close(accepted)
		<-stop
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := newEncoder(conn, time.Second)
	if err != nil {
		t.Fatalf("newEncoder: %v", err)
	}
	<-accepted

	// Break the connection: writeMsg still buffers into framestream successfully,
	// but flush() (the real socket write) fails, exercising the flush-error branch.
	conn.Close()

	l := newListener("tcp", "127.0.0.1:0")
	c := &client{conn: conn, enc: enc, quit: make(chan struct{})}
	l.clients[c] = struct{}{}

	// Message.Type is a required proto field; without it proto.Marshal fails in
	// writeMsg and we never reach the flush-error branch this test exercises.
	dnstapType := tap.Dnstap_MESSAGE
	msgType := tap.Message_CLIENT_QUERY
	payload := &tap.Dnstap{Type: &dnstapType, Message: &tap.Message{Type: &msgType}}

	done := make(chan struct{})
	go func() {
		l.Dnstap(payload)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Dnstap deadlocked: removeClient takes clientsMu.Lock while Dnstap holds RLock")
	}
}

func TestListenerRemoveClient(t *testing.T) {
	l := newListener("tcp", "127.0.0.1:16002")

	mockConn := &mockConn{writes: [][]byte{}}

	c := &client{
		conn: mockConn,
		enc:  nil, // Skip encoder for simplicity
		quit: make(chan struct{}),
	}

	l.clients[c] = struct{}{}

	if len(l.clients) != 1 {
		t.Error("Expected 1 client")
	}

	l.removeClient(c)

	if len(l.clients) != 0 {
		t.Error("Expected 0 clients after removal")
	}

	// Verify quit channel is closed
	select {
	case <-c.quit:
		// Good, channel is closed
	default:
		t.Error("Expected quit channel to be closed")
	}
}

// mockConn implements net.Conn for testing
type mockConn struct {
	writes [][]byte
	closed bool
}

func (m *mockConn) Read(_ []byte) (n int, err error) {
	return 0, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	if m.closed {
		return 0, net.ErrClosed
	}
	// Copy the data to avoid issues with buffer reuse
	data := make([]byte, len(b))
	copy(data, b)
	m.writes = append(m.writes, data)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 16000}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000}
}

func (m *mockConn) SetDeadline(_ time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

func TestListenerClose(t *testing.T) {
	l := newListener("tcp", "127.0.0.1:16003")

	// Add some mock clients
	for range 3 {
		mockConn := &mockConn{writes: [][]byte{}}
		c := &client{
			conn: mockConn,
			enc:  nil, // Skip encoder for simplicity
			quit: make(chan struct{}),
		}
		l.clients[c] = struct{}{}
	}

	if len(l.clients) != 3 {
		t.Errorf("Expected 3 clients, got %d", len(l.clients))
	}

	l.close()

	if len(l.clients) != 0 {
		t.Errorf("Expected 0 clients after close, got %d", len(l.clients))
	}

	// Verify quit channel is closed
	select {
	case <-l.quit:
		// Good, channel is closed
	default:
		t.Error("Expected listener quit channel to be closed")
	}
}
