package shed

import (
	"context"
	"testing"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newShed constructs a Shed whose package-level registry entries (and
// writer goroutines) are removed after the test.
func newShed(t *testing.T, next plugin.Handler) *Shed {
	t.Helper()
	s := &Shed{Next: next}
	t.Cleanup(func() { _ = s.shutdown() })
	return s
}

func msg() *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	return m
}

// packedReply is what miekg/dns's WriteMsg hands the decorated writer.
func packedReply(t *testing.T) []byte {
	t.Helper()
	m := new(dns.Msg)
	m.SetReply(msg())
	data, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ctxFor(srv *dnsserver.Server) context.Context {
	return context.WithValue(context.Background(), dnsserver.Key{}, srv)
}

// answering is a Next handler that writes a response.
func answering() plugin.Handler {
	return plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		if err := w.WriteMsg(m); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})
}

// blockingWriter parks the writer goroutine in a raw Write until release is
// closed.
type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingWriter) Write(p []byte) (int, error) {
	b.entered <- struct{}{}
	<-b.release
	return len(p), nil
}

// fillStack parks srv's writer goroutine and fills the stack through the
// production decorator; cleanup releases the writer.
func fillStack(t *testing.T, s *Shed, srv *dnsserver.Server) *socketState {
	t.Helper()
	dec := s.decorateWriterFactory(srv)
	v, ok := registry.Load(srv)
	if !ok {
		t.Fatal("decorator factory must register the socket's state")
	}
	st := v.(*socketState)
	bw := &blockingWriter{
		entered: make(chan struct{}, stackDepth+2),
		release: make(chan struct{}),
	}
	t.Cleanup(func() { close(bw.release) })
	data := packedReply(t)
	// First push is popped by the writer, which parks in the raw Write.
	if _, err := dec(bw).Write(data); err != nil {
		t.Fatal(err)
	}
	<-bw.entered
	for !st.stack.full() {
		if _, err := dec(bw).Write(data); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestNoServerInContextFailsOpen(t *testing.T) {
	s := newShed(t, answering())
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := s.ServeDNS(context.Background(), rec, msg()); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil {
		t.Fatal("expected a response without a dnsserver in the context")
	}
}

func TestUnregisteredSocketFailsOpen(t *testing.T) {
	s := newShed(t, answering())
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	// The server carried by the context was never registered by the
	// decorator factory (e.g. a straggler after a reload swept it).
	if _, err := s.ServeDNS(ctxFor(&dnsserver.Server{}), rec, msg()); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil {
		t.Fatal("expected a response for an unregistered socket")
	}
}

func TestTCPPassesThrough(t *testing.T) {
	s := newShed(t, answering())
	srv := &dnsserver.Server{}
	fillStack(t, s, srv)
	rec := dnstest.NewRecorder(&test.ResponseWriter{TCP: true})
	// Even with the socket's stack full, TCP is never shed.
	if _, err := s.ServeDNS(ctxFor(srv), rec, msg()); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil {
		t.Fatal("expected a response over TCP")
	}
}

func TestCoupledShedWhenStackFull(t *testing.T) {
	s := newShed(t, answering())
	srv := &dnsserver.Server{}
	st := fillStack(t, s, srv)

	before := testutil.ToFloat64(st.droppedQuery)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	rcode, err := s.ServeDNS(ctxFor(srv), rec, msg())
	if err != nil {
		t.Fatal(err)
	}
	if rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want RcodeSuccess (silent drop)", rcode)
	}
	if rec.Msg != nil {
		t.Error("a shed query must not be answered")
	}
	if got := testutil.ToFloat64(st.droppedQuery) - before; got != 1 {
		t.Errorf("dropped_total{reason=%q} increment = %v, want 1", "query", got)
	}
}

func TestPassesThroughWhenNotFull(t *testing.T) {
	s := newShed(t, answering())
	srv := &dnsserver.Server{}
	s.decorateWriterFactory(srv)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := s.ServeDNS(ctxFor(srv), rec, msg()); err != nil {
		t.Fatal(err)
	}
	if rec.Msg == nil {
		t.Fatal("expected a response while the stack has room")
	}
}
