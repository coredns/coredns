package proxy

import (
	"runtime"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
)

func TestCached(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestCached", s.Addr)
	tr.Start()
	defer tr.Stop()

	c1, cache1, _ := tr.Dial("udp")
	c2, cache2, _ := tr.Dial("udp")

	if cache1 || cache2 {
		t.Errorf("Expected non-cached connection")
	}

	tr.Yield(c1)
	tr.Yield(c2)
	c3, cached3, _ := tr.Dial("udp")
	if !cached3 {
		t.Error("Expected cached connection (c3)")
	}
	// FIFO: first yielded (c1) should be first out
	if c1 != c3 {
		t.Error("Expected c1 == c3 (FIFO order)")
	}

	tr.Yield(c3)

	// dial another protocol
	c4, cached4, _ := tr.Dial("tcp")
	if cached4 {
		t.Errorf("Expected non-cached connection (c4)")
	}
	tr.Yield(c4)
}

func TestCleanupByTimer(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestCleanupByTimer", s.Addr)
	tr.SetExpire(100 * time.Millisecond)
	tr.Start()
	defer tr.Stop()

	c1, _, _ := tr.Dial("udp")
	c2, _, _ := tr.Dial("udp")
	tr.Yield(c1)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c2)

	time.Sleep(120 * time.Millisecond)
	c3, cached, _ := tr.Dial("udp")
	if cached {
		t.Error("Expected non-cached connection (c3)")
	}
	tr.Yield(c3)

	time.Sleep(120 * time.Millisecond)
	c4, cached, _ := tr.Dial("udp")
	if cached {
		t.Error("Expected non-cached connection (c4)")
	}
	tr.Yield(c4)
}

func TestCleanupAll(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestCleanupAll", s.Addr)

	c1, _ := dns.DialTimeout("udp", tr.addr, maxDialTimeout)
	c2, _ := dns.DialTimeout("udp", tr.addr, maxDialTimeout)
	c3, _ := dns.DialTimeout("udp", tr.addr, maxDialTimeout)

	now := time.Now()
	tr.conns[typeUDP] = []*persistConn{
		{c: c1, created: now, used: now},
		{c: c2, created: now, used: now},
		{c: c3, created: now, used: now},
	}

	if len(tr.conns[typeUDP]) != 3 {
		t.Error("Expected 3 connections")
	}
	tr.cleanup(true)

	if len(tr.conns[typeUDP]) > 0 {
		t.Error("Expected no cached connections")
	}
}

func TestMaxIdleConns(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestMaxIdleConns", s.Addr)
	tr.SetMaxIdleConns(2) // Limit to 2 connections per type
	tr.Start()
	defer tr.Stop()

	// Dial 3 connections
	c1, _, _ := tr.Dial("udp")
	c2, _, _ := tr.Dial("udp")
	c3, _, _ := tr.Dial("udp")

	// Yield all 3
	tr.Yield(c1)
	tr.Yield(c2)
	tr.Yield(c3) // This should be discarded (pool full)

	// Check pool size is capped at 2
	tr.mu.Lock()
	poolSize := len(tr.conns[typeUDP])
	tr.mu.Unlock()

	if poolSize != 2 {
		t.Errorf("Expected pool size 2, got %d", poolSize)
	}

	// Verify we get the first 2 back (FIFO)
	d1, cached1, _ := tr.Dial("udp")
	d2, cached2, _ := tr.Dial("udp")
	_, cached3, _ := tr.Dial("udp")

	if !cached1 || !cached2 {
		t.Error("Expected first 2 dials to be cached")
	}
	if cached3 {
		t.Error("Expected 3rd dial to be non-cached (pool was limited to 2)")
	}
	if d1 != c1 || d2 != c2 {
		t.Error("Expected FIFO order: d1==c1, d2==c2")
	}
}

func TestMaxIdleConnsUnlimited(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestMaxIdleConnsUnlimited", s.Addr)
	// maxIdleConns defaults to 0 (unlimited)
	tr.Start()
	defer tr.Stop()

	// Dial and yield 5 connections
	conns := make([]*persistConn, 5)
	for i := range conns {
		conns[i], _, _ = tr.Dial("udp")
	}
	for _, c := range conns {
		tr.Yield(c)
	}

	// Check all 5 are in pool
	tr.mu.Lock()
	poolSize := len(tr.conns[typeUDP])
	tr.mu.Unlock()

	if poolSize != 5 {
		t.Errorf("Expected pool size 5 (unlimited), got %d", poolSize)
	}
}

func TestYieldAfterStop(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestYieldAfterStop", s.Addr)
	tr.Start()

	// Dial a connection while transport is running
	c1, _, err := tr.Dial("udp")
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Stop the transport
	tr.Stop()

	// Give cleanup goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// Yield the connection after stop - should close it, not pool it
	tr.Yield(c1)

	// Verify pool is empty (connection was closed, not added)
	tr.mu.Lock()
	poolSize := len(tr.conns[typeUDP])
	tr.mu.Unlock()

	if poolSize != 0 {
		t.Errorf("Expected pool size 0 after stop, got %d", poolSize)
	}
}

// TestMaxAgeExpireByCreation verifies that a connection is rejected when its
// creation time exceeds max_age, even if it was recently yielded (fresh used time).
// This guards against the FIFO rotation bug where used time is continually
// refreshed, preventing connections from expiring by idle-timeout alone.
func TestMaxAgeExpireByCreation(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestMaxAgeExpireByCreation", s.Addr)
	tr.SetExpire(10 * time.Second)       // long idle-timeout: would not expire the connection
	tr.SetMaxAge(100 * time.Millisecond) // short max-age: should close old connection
	tr.Start()
	defer tr.Stop()

	// Inject a connection whose creation time is past max_age but whose used
	// time is fresh, simulating a FIFO-rotated connection that is never idle.
	oldConn, err := dns.DialTimeout("udp", tr.addr, maxDialTimeout)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	pc := &persistConn{
		c:       oldConn,
		created: time.Now().Add(-200 * time.Millisecond), // 2x max-age: should be closed
		used:    time.Now(),                              // freshly used: idle-timeout would pass
	}
	tr.mu.Lock()
	tr.conns[typeUDP] = []*persistConn{pc}
	tr.mu.Unlock()

	_, cached, _ := tr.Dial("udp")
	if cached {
		t.Error("connection should be closed by max_age, not reused despite fresh used time")
	}
}

// TestMaxAgeFIFORotation verifies that connections in a FIFO pool are closed by
// max_age even when continuously rotated (which refreshes their used timestamps).
// Regression test for Scale up: new upstream pods should receive traffic after
// existing connections exceed max_age, regardless of request rate.
func TestMaxAgeFIFORotation(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("TestMaxAgeFIFORotation", s.Addr)
	tr.SetExpire(10 * time.Second)       // long idle-timeout: FIFO rotation keeps connections alive
	tr.SetMaxAge(100 * time.Millisecond) // max-age: connections must be closed by creation age
	tr.Start()
	defer tr.Stop()

	// Inject 3 connections old by creation time but with fresh used timestamps,
	// simulating active FIFO rotation where idle-timeout never triggers.
	tr.mu.Lock()
	for range 3 {
		c, err := dns.DialTimeout("udp", tr.addr, maxDialTimeout)
		if err != nil {
			tr.mu.Unlock()
			t.Fatalf("Failed to dial: %v", err)
		}
		tr.conns[typeUDP] = append(tr.conns[typeUDP], &persistConn{
			c:       c,
			created: time.Now().Add(-200 * time.Millisecond), // exceeds max-age
			used:    time.Now(),                              // fresh: idle-timeout would pass
		})
	}
	tr.mu.Unlock()

	// All 3 connections must be rejected by max_age despite fresh used timestamps.
	for i := range 3 {
		_, cached, _ := tr.Dial("udp")
		if cached {
			t.Errorf("Dial %d: connection should be closed by max_age (FIFO rotation must not prevent max-age expiry)", i+1)
		}
	}
}

func BenchmarkYield(b *testing.B) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport("BenchmarkYield", s.Addr)
	tr.Start()
	defer tr.Stop()

	c, _, _ := tr.Dial("udp")

	b.ReportAllocs()

	for b.Loop() {
		tr.Yield(c)
		// Simulate FIFO consumption: remove from front
		tr.mu.Lock()
		if len(tr.conns[typeUDP]) > 0 {
			tr.conns[typeUDP] = tr.conns[typeUDP][1:]
		}
		tr.mu.Unlock()
		runtime.Gosched()
	}
}
