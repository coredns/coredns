package shed

import (
	"sync"
	"sync/atomic"

	"github.com/coredns/coredns/core/dnsserver"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// decorateWriterFactory is installed as Config.UDPDecorateWriterFunc by
// setup. dnsserver's ServePacket calls it once per UDP listener socket,
// before the socket serves its first packet, so the socket's state and
// writer goroutine exist before ServeDNS ever looks them up. miekg/dns then
// applies the returned dns.DecorateWriter once per packet, wrapping the
// response writer.
func (s *Shed) decorateWriterFactory(srv *dnsserver.Server) dns.DecorateWriter {
	st := s.mintState(srv)
	return func(w dns.Writer) dns.Writer {
		return &stackWriter{stack: st.stack, inner: w}
	}
}

// stackWriter is the per-packet transport wrapper. miekg/dns runs every
// message transform (including TSIG) before handing Write the packed bytes,
// so the deferred operation is precisely the serialized syscall.
type stackWriter struct {
	stack *respStack
	inner dns.Writer // the raw response writer; its Write is the terminal syscall
}

// Write pushes the packed bytes and reports success: from here on "written"
// means "queued for the socket's writer goroutine". The pushed slice is
// exclusively owned — miekg/dns packs each response into a fresh allocation.
func (sw *stackWriter) Write(data []byte) (int, error) {
	if sw.stack.push(pendingResp{w: sw.inner, data: data}) {
		sw.stack.dropped.Inc()
	}
	return len(data), nil
}

// pendingResp is one captured response awaiting the socket's writer
// goroutine: the packed bytes and the raw writer that puts them on the wire.
type pendingResp struct {
	w    dns.Writer
	data []byte
}

// respStack is a per-socket bounded ring of pending responses with a single
// insertion cursor and no head index: entries occupy the size slots before
// next (mod depth), so when the ring is full the slot at next holds the
// oldest entry and pushing over it is the eviction.
type respStack struct {
	dropped prometheus.Counter // responses evicted, write-failed, or pushed after close

	mu     sync.Mutex
	buf    []pendingResp // ring; len(buf) is the fixed depth
	next   int           // index of the next push
	size   int           // occupied slots
	closed bool          // set by close; pushes are rejected from then on

	n      atomic.Int64  // size mirror for the lock-free full() check
	notify chan struct{} // cap 1: writer wake-up
	stop   chan struct{} // closed on shutdown
}

func newRespStack(depth int, dropped prometheus.Counter) *respStack {
	return &respStack{
		dropped: dropped,
		buf:     make([]pendingResp, depth),
		notify:  make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
}

// push adds p as the newest entry, evicting the oldest when full. Never
// blocks. Reports whether a response was dropped as a result: the evicted
// oldest, or — on a closed stack — p itself.
func (rs *respStack) push(p pendingResp) (dropped bool) {
	rs.mu.Lock()
	switch {
	case rs.closed:
		rs.mu.Unlock()
		return true
	case rs.size == len(rs.buf):
		dropped = true // the slot at next holds the oldest entry
	default:
		rs.size++
	}
	rs.buf[rs.next] = p
	rs.next = (rs.next + 1) % len(rs.buf)
	rs.n.Store(int64(rs.size))
	rs.mu.Unlock()
	select {
	case rs.notify <- struct{}{}:
	default:
	}
	return dropped
}

// pop removes and returns the newest entry.
func (rs *respStack) pop() (pendingResp, bool) {
	rs.mu.Lock()
	if rs.size == 0 {
		rs.mu.Unlock()
		return pendingResp{}, false
	}
	rs.next = (rs.next - 1 + len(rs.buf)) % len(rs.buf)
	p := rs.buf[rs.next]
	rs.buf[rs.next] = pendingResp{} // release the response bytes
	rs.size--
	rs.n.Store(int64(rs.size))
	rs.mu.Unlock()
	return p, true
}

// full is the lock-free view used by the coupled-shed predicate.
func (rs *respStack) full() bool { return rs.n.Load() >= int64(len(rs.buf)) }

// close stops the writer goroutine — it drains whatever is stacked, then
// exits — and rejects any straggler pushes.
func (rs *respStack) close() {
	rs.mu.Lock()
	rs.closed = true
	rs.mu.Unlock()
	close(rs.stop)
}

// waitNonempty blocks until the stack has work, or reports false once the
// stack is closed and empty. The re-check on the stop arm matters: the
// select may pick stop over a pending notify, but entries accepted before
// the close must still be served — closed guarantees no new pushes, so the
// drain terminates.
func (rs *respStack) waitNonempty() bool {
	if rs.n.Load() > 0 {
		return true
	}
	select {
	case <-rs.notify:
		return true
	case <-rs.stop:
		return rs.n.Load() > 0
	}
}

// writerLoop is the socket's single writer: it waits for pending responses,
// pops the one that is newest at write time, and writes it to the wire.
func (rs *respStack) writerLoop() {
	for {
		if !rs.waitNonempty() {
			return
		}
		if p, ok := rs.pop(); ok {
			rs.write(p)
		}
	}
}

// write performs the deferred raw write; a response that fails to reach the
// wire is a counted drop. A writer panic must not kill the process — that is
// the failure class this plugin removes — so it is recovered, like
// dnsserver does for synchronous writes.
func (rs *respStack) write(p pendingResp) {
	defer func() {
		if rec := recover(); rec != nil {
			rs.dropped.Inc()
			log.Errorf("Recovered panic in shed writer: %v", rec)
		}
	}()
	if _, err := p.w.Write(p.data); err != nil {
		rs.dropped.Inc()
		log.Debugf("Deferred response write failed: %s", err)
	}
}
