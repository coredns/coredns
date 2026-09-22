package shed

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"

	"github.com/miekg/dns"
)

// chanWriter hands each written payload to a channel — the race-safe way to
// observe the writer goroutine's deferred writes.
type chanWriter struct {
	got chan []byte
}

func (w *chanWriter) Write(p []byte) (int, error) {
	w.got <- p
	return len(p), nil
}

func TestStackEvictsOldestPopsNewest(t *testing.T) {
	rs := newRespStack(3, droppedTotal.WithLabelValues(t.Name(), "response")) // no writer goroutine: pure data structure test
	for i := 1; i <= 5; i++ {
		dropped := rs.push(pendingResp{data: []byte{byte(i)}})
		if want := i > 3; dropped != want {
			t.Errorf("push %d: dropped = %v, want %v", i, dropped, want)
		}
	}
	if !rs.full() {
		t.Error("expected full stack after overfilling")
	}
	// 1 and 2 were evicted; the survivors pop newest-first.
	for _, want := range []byte{5, 4, 3} {
		p, ok := rs.pop()
		if !ok || p.data[0] != want {
			t.Fatalf("pop = %v, %v; want entry %d", p.data, ok, want)
		}
	}
	if _, ok := rs.pop(); ok {
		t.Error("expected empty stack")
	}
}

func TestStackCloseRejectsPushDrainsRest(t *testing.T) {
	rs := newRespStack(4, droppedTotal.WithLabelValues(t.Name(), "response"))

	// A stale notify token on an empty open stack wakes the writer, which
	// must tolerate the failed pop (writerLoop's pop-ok check).
	rs.push(pendingResp{data: []byte{9}})
	rs.pop() // pop directly, leaving the push's token buffered
	if !rs.waitNonempty() {
		t.Error("a stale token should report as work")
	}
	if _, ok := rs.pop(); ok {
		t.Error("pop should find nothing behind a stale token")
	}

	rs.push(pendingResp{data: []byte{1}})
	rs.close()
	if !rs.push(pendingResp{data: []byte{2}}) {
		t.Error("push on closed stack should report a drop")
	}
	// Entries accepted before the close must still be served.
	if !rs.waitNonempty() {
		t.Fatal("waitNonempty should report the pre-close entry")
	}
	if p, ok := rs.pop(); !ok || p.data[0] != 1 {
		t.Fatalf("pop = %v, %v; want pre-close entry", p, ok)
	}
	// The pre-close push's token may still be buffered; drain it so the
	// final wait deterministically takes the stop arm.
	select {
	case <-rs.notify:
	default:
	}
	if rs.waitNonempty() {
		t.Error("waitNonempty should report false once closed and drained")
	}
}

func TestDecoratorCapturesWriteAndWriterWrites(t *testing.T) {
	s := newShed(t, nil)
	srv := &dnsserver.Server{}
	dec := s.decorateWriterFactory(srv)
	if _, ok := registry.Load(srv); !ok {
		t.Fatal("factory should pre-register the socket's state")
	}

	cw := &chanWriter{got: make(chan []byte, 1)}
	data := packedReply(t)
	w := dec(cw)
	if _, ok := w.(*stackWriter); !ok {
		t.Fatalf("decorator returned %T, want *stackWriter", w)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-cw.got:
		m := new(dns.Msg)
		if err := m.Unpack(got); err != nil {
			t.Fatalf("writer goroutine wrote unparseable bytes: %s", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer goroutine never performed the deferred write")
	}
}

// panicWriter panics on its first Write, then counts.
type panicWriter struct {
	writes atomic.Int64
}

func (w *panicWriter) Write(p []byte) (int, error) {
	if w.writes.Add(1) == 1 {
		panic("writer exploded")
	}
	return len(p), nil
}

func TestWriterPanicRecovered(t *testing.T) {
	s := newShed(t, nil)
	srv := &dnsserver.Server{}
	dec := s.decorateWriterFactory(srv)

	pw := &panicWriter{}
	data := packedReply(t)
	if _, err := dec(pw).Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := dec(pw).Write(data); err != nil {
		t.Fatal(err)
	}
	// The writer goroutine must survive the first write's panic and still
	// perform the second.
	deadline := time.Now().Add(5 * time.Second)
	for pw.writes.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("writer performed %d writes, want 2 (goroutine died on panic?)", pw.writes.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestShutdownIsInstanceScoped(t *testing.T) {
	old := newShed(t, nil)
	cur := newShed(t, nil)
	oldSrv, newSrv := &dnsserver.Server{}, &dnsserver.Server{}
	old.decorateWriterFactory(oldSrv)
	cur.decorateWriterFactory(newSrv)

	if err := old.shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Load(oldSrv); ok {
		t.Error("old instance's entry should be swept")
	}
	if _, ok := registry.Load(newSrv); !ok {
		t.Error("new instance's entry must survive the old instance's shutdown")
	}
}
