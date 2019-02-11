package dnsproxy

import (
	"context"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestDnsProxy(t *testing.T) {
	t.Parallel()
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	metrics := metrics.New()
	opts := &DNSOpts{Expire: 2 * time.Second}
	p := New(s.Addr, opts, metrics)
	p.transport.Start()
	defer p.Stop()

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	res, err := p.Query(context.TODO(), state)
	if err != nil {
		t.Error("Expecting a response")
	}
	if res.Answer[0].Header().Name != "example.org." {
		t.Error("Expecting example.org. answer")
	}
}

func TestUDPDnsProxy(t *testing.T) {
	t.Parallel()
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	metrics := metrics.New()
	opts := &DNSOpts{PreferUDP: true, Expire: 2 * time.Second}
	p := New(s.Addr, opts, metrics)
	p.transport.Start()
	defer p.Stop()

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	res, err := p.Query(context.TODO(), state)
	if err != nil {
		t.Error("Expecting a response")
	}
	if res.Answer[0].Header().Name != "example.org." {
		t.Error("Expecting example.org. answer")
	}
}

func TestTCPDnsProxy(t *testing.T) {
	t.Parallel()
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	metrics := metrics.New()
	opts := &DNSOpts{ForceTCP: true, Expire: 2 * time.Second}
	p := New(s.Addr, opts, metrics)
	p.transport.Start()
	defer p.Stop()

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	state := request.Request{W: &test.ResponseWriter{}, Req: m}
	res, err := p.Query(context.TODO(), state)
	if err != nil {
		t.Error("Expecting a response")
	}
	if res.Answer[0].Header().Name != "example.org." {
		t.Error("Expecting example.org. answer")
	}
}

func TestUnavailableDNSProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	opts := &DNSOpts{Expire: 1 * time.Second}
	p := New("1.2.3.4:5", opts, metrics)
	p.transport.Start()
	defer p.Stop()

	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	if _, err := p.Query(context.TODO(), state); err == nil {
		t.Errorf("Expecting an unavailable error when querying gRPC client with invalid hostname : %s", err)
	}
}

func TestHealthcheckGrpcProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	opts := &DNSOpts{Expire: 1 * time.Second}
	p := New("1.2.3.4:5", opts, metrics)
	p.Start(1 * time.Millisecond)
	defer p.Stop()

	p.Healthcheck()
	time.Sleep(2500 * time.Millisecond)
	if !p.Down(1) {
		t.Error("Expecting a down service")
	}
}

func TestTLSDnsProxy(t *testing.T) {
	t.Parallel()
	//TODO
}

const hcInterval = 500 * time.Millisecond
