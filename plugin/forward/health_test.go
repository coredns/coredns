package forward

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestHealth(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	i := uint32(0)
	q := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if atomic.LoadUint32(&q) == 0 { //drop the first query to trigger health-checking
			atomic.AddUint32(&q, 1)
			return
		}
		if r.Question[0].Name == "." && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealth", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	f := New()
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==true to be %d, got %d", 1, i1)
	}
}

func TestHealthTCP(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	i := uint32(0)
	q := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if atomic.LoadUint32(&q) == 0 { //drop the first query to trigger health-checking
			atomic.AddUint32(&q, 1)
			return
		}
		if r.Question[0].Name == "." && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealthTCP", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetTCPTransport()
	f := New()
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{TCP: true}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==true to be %d, got %d", 1, i1)
	}
}

func TestHealthNoRecursion(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	i := uint32(0)
	q := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if atomic.LoadUint32(&q) == 0 { //drop the first query to trigger health-checking
			atomic.AddUint32(&q, 1)
			return
		}
		if r.Question[0].Name == "." && r.RecursionDesired == false {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealthNoRecursion", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetRecursionDesired(false)
	f := New()
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with RecursionDesired==false to be %d, got %d", 1, i1)
	}
}

func TestHealthTimeout(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	i := uint32(0)
	q := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == "." {
			// health check, answer
			atomic.AddUint32(&i, 1)
			ret := new(dns.Msg)
			ret.SetReply(r)
			w.WriteMsg(ret)
			return
		}
		if atomic.LoadUint32(&q) == 0 { //drop only first query
			atomic.AddUint32(&q, 1)
			return
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealthTimeout", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	f := New()
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks to be %d, got %d", 1, i1)
	}
}

func TestHealthMaxFails(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond
	//,hcInterval = 10 * time.Millisecond

	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		// timeout
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealthMaxFails", s.Addr, transport.DNS)
	p.SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	f := New()
	f.hcInterval = 10 * time.Millisecond
	f.maxfails = 2
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(100 * time.Millisecond)
	fails := p.Fails()
	if !p.Down(f.maxfails) {
		t.Errorf("Expected Proxy fails to be greater than %d, got %d", f.maxfails, fails)
	}
}

func TestHealthNoMaxFails(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	i := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Question[0].Name == "." {
			// health check, answer
			atomic.AddUint32(&i, 1)
			ret := new(dns.Msg)
			ret.SetReply(r)
			w.WriteMsg(ret)
		}
	})
	defer s.Close()

	p := proxy.NewProxy("TestHealthNoMaxFails", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	f := New()
	f.maxfails = 0
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 0 {
		t.Errorf("Expected number of health checks to be %d, got %d", 0, i1)
	}
}

func TestHealthDomain(t *testing.T) {
	defaultTimeout = 10 * time.Millisecond

	hcDomain := "example.org."
	i := uint32(0)
	q := uint32(0)
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		if atomic.LoadUint32(&q) == 0 { //drop the first query to trigger health-checking
			atomic.AddUint32(&q, 1)
			return
		}
		if r.Question[0].Name == hcDomain && r.RecursionDesired == true {
			atomic.AddUint32(&i, 1)
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		w.WriteMsg(ret)
	})
	defer s.Close()
	p := proxy.NewProxy("TestHealthDomain", s.Addr, transport.DNS)
	p.GetHealthchecker().SetReadTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetWriteTimeout(10 * time.Millisecond)
	p.GetHealthchecker().SetDomain(hcDomain)
	f := New()
	f.SetProxy(p)
	defer f.OnShutdown()

	req := new(dns.Msg)
	req.SetQuestion(".", dns.TypeNS)

	f.ServeDNS(context.TODO(), &test.ResponseWriter{}, req)

	time.Sleep(20 * time.Millisecond)
	i1 := atomic.LoadUint32(&i)
	if i1 != 1 {
		t.Errorf("Expected number of health checks with Domain==%s to be %d, got %d", hcDomain, 1, i1)
	}
}
