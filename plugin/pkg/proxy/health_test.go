package proxy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
)

func TestHealth(t *testing.T) {
	i := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == "." && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	hc := NewHealthChecker("TestHealth", transport.DNS, true, "")
	hc.SetReadTimeout(10 * time.Millisecond)
	hc.SetWriteTimeout(10 * time.Millisecond)

	p := NewProxy("TestHealth", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	err := hc.Check(p)
	if err != nil {
		t.Errorf("check failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==true to be %d, got %d", 1, i1)
	}
}

func TestHealthTCP(t *testing.T) {
	i := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == "." && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	hc := NewHealthChecker("TestHealthTCP", transport.DNS, true, "")
	hc.SetTCPTransport()
	hc.SetReadTimeout(10 * time.Millisecond)
	hc.SetWriteTimeout(10 * time.Millisecond)

	p := NewProxy("TestHealthTCP", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	err := hc.Check(p)
	if err != nil {
		t.Errorf("check failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==true to be %d, got %d", 1, i1)
	}
}

func TestHealthNoRecursion(t *testing.T) {
	i := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == "." && r.RecursionDesired == false {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	hc := NewHealthChecker("TestHealthNoRecursion", transport.DNS, false, "")
	hc.SetReadTimeout(10 * time.Millisecond)
	hc.SetWriteTimeout(10 * time.Millisecond)

	p := NewProxy("TestHealthNoRecursion", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	err := hc.Check(p)
	if err != nil {
		t.Errorf("check failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==false to be %d, got %d", 1, i1)
	}
}

func TestHealthTimeout(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		// timeout
	})
	defer s.Close()

	hc := NewHealthChecker("TestHealthTimeout", transport.DNS, false, "")
	hc.SetReadTimeout(10 * time.Millisecond)
	hc.SetWriteTimeout(10 * time.Millisecond)

	p := NewProxy("TestHealthTimeout", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	err := hc.Check(p)
	if err == nil {
		t.Errorf("expected error")
	}
}

func TestHealthDomain(t *testing.T) {
	hcDomain := "example.org."

	i := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == hcDomain && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	hc := NewHealthChecker("TestHealthDomain", transport.DNS, true, hcDomain)
	hc.SetReadTimeout(10 * time.Millisecond)
	hc.SetWriteTimeout(10 * time.Millisecond)

	p := NewProxy("TestHealthDomain", s.Addr, transport.DNS)
	p.readTimeout = 10 * time.Millisecond
	err := hc.Check(p)
	if err != nil {
		t.Errorf("check failed: %v", err)
	}

	time.Sleep(12 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with Domain==%s to be %d, got %d", hcDomain, 1, i1)
	}
}
