package plugin

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"
)

// mockResponseWriter implements dns.ResponseWriter for testing
type mockResponseWriter struct {
	msg *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}
func (m *mockResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 40212}
}
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error { m.msg = msg; return nil }
func (m *mockResponseWriter) Write([]byte) (int, error)   { return 0, nil }
func (m *mockResponseWriter) Close() error                { return nil }
func (m *mockResponseWriter) TsigStatus() error           { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)         {}
func (m *mockResponseWriter) Hijack()                     {}

// mockPluginTracker implements PluginTracker for testing
type mockPluginTracker struct {
	mockResponseWriter
	plugin string
}

func (m *mockPluginTracker) SetPlugin(name string) { m.plugin = name }
func (m *mockPluginTracker) GetPlugin() string     { return m.plugin }

// mockHandler implements Handler for testing
type mockHandler struct {
	name      string
	writeMsg  bool
	returnErr error
}

func (m *mockHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if m.writeMsg {
		resp := new(dns.Msg)
		resp.SetReply(r)
		w.WriteMsg(resp)
	}
	if m.returnErr != nil {
		return dns.RcodeServerFailure, m.returnErr
	}
	return dns.RcodeSuccess, nil
}

func (m *mockHandler) Name() string { return m.name }

func TestPluginWriter_WriteMsg_SetsPlugin(t *testing.T) {
	tracker := &mockPluginTracker{}
	pw := &pluginWriter{ResponseWriter: tracker, plugin: "whoami"}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	err := pw.WriteMsg(msg)
	if err != nil {
		t.Fatalf("WriteMsg returned error: %v", err)
	}

	if tracker.plugin != "whoami" {
		t.Errorf("Expected plugin to be 'whoami', got %q", tracker.plugin)
	}
}

func TestPluginWriter_Write_SetsPlugin(t *testing.T) {
	tracker := &mockPluginTracker{}
	pw := &pluginWriter{ResponseWriter: tracker, plugin: "forward"}

	_, err := pw.Write([]byte("test"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if tracker.plugin != "forward" {
		t.Errorf("Expected plugin to be 'forward', got %q", tracker.plugin)
	}
}

func TestPluginWriter_NonTracker_NoError(t *testing.T) {
	// When the underlying writer doesn't implement PluginTracker,
	// WriteMsg should still work without error
	mock := &mockResponseWriter{}
	pw := &pluginWriter{ResponseWriter: mock, plugin: "whoami"}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	err := pw.WriteMsg(msg)
	if err != nil {
		t.Fatalf("WriteMsg returned error: %v", err)
	}

	if mock.msg == nil {
		t.Error("Expected message to be written")
	}
}

func TestNextOrFailure_WrapsWithPluginWriter(t *testing.T) {
	tracker := &mockPluginTracker{}
	handler := &mockHandler{name: "testplugin", writeMsg: true}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := NextOrFailure("caller", handler, context.Background(), tracker, req)
	if err != nil {
		t.Fatalf("NextOrFailure returned error: %v", err)
	}

	// The handler should have written a message, which should have set the plugin
	if tracker.plugin != "testplugin" {
		t.Errorf("Expected plugin to be 'testplugin', got %q", tracker.plugin)
	}
}

func TestNextOrFailure_NilHandler(t *testing.T) {
	mock := &mockResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	rcode, err := NextOrFailure("caller", nil, context.Background(), mock, req)
	if err == nil {
		t.Error("Expected error for nil handler")
	}
	if rcode != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure, got %d", rcode)
	}
}

func TestPluginWriter_DelegatesMethods(t *testing.T) {
	mock := &mockResponseWriter{}
	pw := &pluginWriter{ResponseWriter: mock, plugin: "test"}

	// Test LocalAddr
	if pw.LocalAddr() == nil {
		t.Error("LocalAddr should not return nil")
	}

	// Test RemoteAddr
	if pw.RemoteAddr() == nil {
		t.Error("RemoteAddr should not return nil")
	}

	// Test Close
	if err := pw.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Test TsigStatus
	if err := pw.TsigStatus(); err != nil {
		t.Errorf("TsigStatus returned error: %v", err)
	}

	// Test TsigTimersOnly (should not panic)
	pw.TsigTimersOnly(true)

	// Test Hijack (should not panic)
	pw.Hijack()
}
