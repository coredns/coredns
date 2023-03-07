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
	hcReadTimeout = 10 * time.Millisecond
	hcWriteTimeout = 10 * time.Millisecond
	readTimeout = 10 * time.Millisecond

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

	hc := NewHealthChecker(transport.DNS, true, "")

	p := NewProxy(s.Addr, transport.DNS)
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
	hcReadTimeout = 10 * time.Millisecond
	hcWriteTimeout = 10 * time.Millisecond
	readTimeout = 10 * time.Millisecond

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

	hc := NewHealthChecker(transport.DNS, true, "")
	hc.SetTCPTransport()

	p := NewProxy(s.Addr, transport.DNS)
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
	hcReadTimeout = 10 * time.Millisecond
	readTimeout = 10 * time.Millisecond
	hcWriteTimeout = 10 * time.Millisecond

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

	hc := NewHealthChecker(transport.DNS, false, "")

	p := NewProxy(s.Addr, transport.DNS)
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
	hcReadTimeout = 10 * time.Millisecond
	hcWriteTimeout = 10 * time.Millisecond
	readTimeout = 10 * time.Millisecond

	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		// timeout
	})
	defer s.Close()

	hc := NewHealthChecker(transport.DNS, false, "")

	p := NewProxy(s.Addr, transport.DNS)
	err := hc.Check(p)
	if err == nil {
		t.Errorf("expected error")
	}
}

func TestHealthDomain(t *testing.T) {
	hcReadTimeout = 10 * time.Millisecond
	readTimeout = 10 * time.Millisecond
	hcWriteTimeout = 10 * time.Millisecond

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

	hc := NewHealthChecker(transport.DNS, true, hcDomain)

	p := NewProxy(s.Addr, transport.DNS)
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
