package dnstap

import (
	"net"
	"testing"
	"time"

	tap "github.com/dnstap/golang-dnstap"
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
