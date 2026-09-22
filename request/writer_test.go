package request

import (
	"crypto/tls"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// mockResponseWriter implements dns.ResponseWriter interface for testing
type mockResponseWriter struct {
	test.ResponseWriter
	lastMsg *dns.Msg
}

func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.lastMsg = msg
	return nil
}

// connStateResponseWriter implements both dns.ResponseWriter and
// dns.ConnectionStater for testing forwarding through ScrubWriter.
type connStateResponseWriter struct {
	test.ResponseWriter
	state *tls.ConnectionState
}

func (c *connStateResponseWriter) ConnectionState() *tls.ConnectionState { return c.state }

func TestScrubWriter(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	req.SetEdns0(4096, true)

	mock := &mockResponseWriter{}
	sw := NewScrubWriter(req, mock)

	// Create a large response message
	resp := new(dns.Msg)
	resp.SetReply(req)

	// Add a lot of records to make it large
	for i := 1; i < 100; i++ {
		resp.Answer = append(resp.Answer, test.A(
			fmt.Sprintf("example.com. 10 IN A 10.0.0.%d", i)))
	}

	// Write the message through ScrubWriter
	err := sw.WriteMsg(resp)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify that ScrubWriter called methods properly
	if mock.lastMsg == nil {
		t.Fatalf("Expected WriteMsg to be called with a message")
	}
}

func TestScrubWriterConnectionStateForwarded(t *testing.T) {
	want := &tls.ConnectionState{ServerName: "example.test"}
	inner := &connStateResponseWriter{state: want}

	sw := NewScrubWriter(new(dns.Msg), inner)

	cs, ok := dns.ResponseWriter(sw).(dns.ConnectionStater)
	if !ok {
		t.Fatal("ScrubWriter does not satisfy dns.ConnectionStater")
	}
	if got := cs.ConnectionState(); got != want {
		t.Errorf("ConnectionState() = %v, want %v", got, want)
	}
}

func TestScrubWriterConnectionStateNilWhenUnsupported(t *testing.T) {
	sw := NewScrubWriter(new(dns.Msg), &mockResponseWriter{})

	if got := sw.ConnectionState(); got != nil {
		t.Errorf("ConnectionState() = %v, want nil when wrapped writer is not a ConnectionStater", got)
	}
}
