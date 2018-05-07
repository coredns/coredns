package forward

import (
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

	tr := newTransport(s.Addr, nil /* no TLS */)
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
	if c2 != c3 {
		t.Error("Expected c2 == c3")
	}

	tr.Yield(c3)

	// dial another protocol
	c4, cached4, _ := tr.Dial("tcp")
	if cached4 {
		t.Errorf("Expected non-cached connection (c4)")
	}
	tr.Yield(c4)
}

func TestCleanup(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	tr := newTransport(s.Addr, nil /* no TLS */)
	defer tr.Stop()

	time.Sleep(10 * time.Millisecond)
	tr.SetExpire(100 * time.Millisecond)

	// partially expired
	c1, _, _ := tr.Dial("udp")
	c2, _, _ := tr.Dial("udp")
	c3, _, _ := tr.Dial("udp")
	c4, _, _ := tr.Dial("udp")
	c5, _, _ := tr.Dial("udp")

	tr.Yield(c1)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c2)
	time.Sleep(60 * time.Millisecond)
	tr.Yield(c3)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c4)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c5)
	time.Sleep(40 * time.Millisecond)
	tr.cleanup(false)
	if l := tr.len(); l != 3 {
		t.Errorf("Expected 3 connections, got %d", l)
	}

	// dial when all expired
	time.Sleep(80 * time.Millisecond)
	if l := tr.len(); l != 3 {
		t.Errorf("Still expected 3 connections, got %d", l)
	}
	var cached bool
	c1, cached, _ = tr.Dial("udp")
	if cached {
		t.Errorf("Expected non-cached connection")
	}
	if l := tr.len(); l != 0 {
		t.Errorf("Expected 0 connections, got %d", l)
	}

	// noone expired
	c2, _, _ = tr.Dial("udp")
	c3, _, _ = tr.Dial("udp")
	c4, _, _ = tr.Dial("udp")
	c5, _, _ = tr.Dial("udp")

	tr.Yield(c1)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c2)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c3)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c4)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c5)
	time.Sleep(10 * time.Millisecond)
	tr.cleanup(false)
	if l := tr.len(); l != 5 {
		t.Errorf("Expected 0 connections, got %d", l)
	}

	// all expired
	c1, _, _ = tr.Dial("udp")
	c2, _, _ = tr.Dial("udp")
	c3, _, _ = tr.Dial("udp")
	c4, _, _ = tr.Dial("udp")
	c5, _, _ = tr.Dial("udp")

	tr.Yield(c1)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c2)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c3)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c4)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c5)
	time.Sleep(120 * time.Millisecond)
	tr.cleanup(false)
	if l := tr.len(); l != 0 {
		t.Errorf("Expected 0 connections, got %d", l)
	}

	// clean all connections
	c1, _, _ = tr.Dial("udp")
	c2, _, _ = tr.Dial("udp")
	c3, _, _ = tr.Dial("udp")

	tr.Yield(c1)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c2)
	time.Sleep(10 * time.Millisecond)
	tr.Yield(c3)
	tr.cleanup(true)
	if l := tr.len(); l != 0 {
		t.Errorf("Expected 0 connections, got %d", l)
	}
}
