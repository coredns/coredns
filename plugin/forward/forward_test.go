package forward

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	m.Response = true
	p := newMockProxy(false, m, nil)
	f.SetProxy(p)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestDownProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	p := newMockProxy(true, &dns.Msg{}, nil)
	f.SetProxy(p)

	m := new(dns.Msg)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err == nil {
		t.Fatal("Expected to receive an error because the proxy is down, but didn't")
	}
}

func TestWrongReplyProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	m := new(dns.Msg)
	p := newMockProxy(false, m, nil)
	f.SetProxy(p)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply with FormErr, but didn't")
	}
	// TODO: check record status is FormErr
}

func TestNoAnswerProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	res := &dns.Msg{}
	res.SetRcode(res, dns.RcodeNameError)
	p := newMockProxy(false, res, nil)
	f.SetProxy(p)

	m := new(dns.Msg)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestErrorResponseProxy(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	p := newMockProxy(false, &dns.Msg{}, errors.New(""))
	f.SetProxy(p)

	m := new(dns.Msg)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestProxies(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	down := false
	m := new(dns.Msg)
	m.SetQuestion("", 1)
	m.Response = true
	for i := 0; i < 10; i++ {
		p := newMockProxy(down, m, nil)
		f.SetProxy(p)
	}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestDownProxies(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	for i := 0; i < 10; i++ {
		p := newMockProxy(true, &dns.Msg{}, nil)
		f.SetProxy(p)
	}

	m := new(dns.Msg)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err == nil {
		t.Fatal("Expected to receive an error because proxies are down, but didn't")
	}
}

func TestRandomDownProxies(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	m.Response = true
	for i := 0; i < 10; i++ {
		down := rand.Float32() < 0.5
		p := newMockProxy(down, m, nil)
		f.SetProxy(p)
	}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestRandomDownNoAnswerProxies(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	m := new(dns.Msg)
	m.SetQuestion("", 1)
	m.Response = true
	for i := 0; i < 10; i++ {
		down := rand.Float32() < 0.5
		if rand.Float32() < 0.5 {
			m.SetRcode(m, dns.RcodeNameError)
		}
		p := newMockProxy(down, m, nil)
		f.SetProxy(p)
	}

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

func TestErrorResponseProxies(t *testing.T) {
	t.Parallel()
	metrics := metrics.New()
	f := New(metrics)

	for i := 0; i < 10; i++ {
		p := newMockProxy(false, &dns.Msg{}, errors.New(""))
		f.SetProxy(p)
	}

	m := new(dns.Msg)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
}

type MockProxy struct {
	fails uint32
	down  bool
	res   *dns.Msg
	err   error
}

func (p *MockProxy) Query(ctx context.Context, state request.Request) (*dns.Msg, error) {
	return p.res, p.err
}
func (p *MockProxy) Healthcheck()                 {}
func (p *MockProxy) Down(maxfails uint32) bool    { return p.down }
func (p *MockProxy) Stop()                        {}
func (p *MockProxy) Start(duration time.Duration) {}
func (p *MockProxy) Address() string              { return "" }
func (p *MockProxy) FailsNum() *uint32            { return &p.fails }

func newMockProxy(down bool, res *dns.Msg, err error) *MockProxy {
	return &MockProxy{
		fails: 0,
		down:  down,
		res:   res,
		err:   err,
	}
}
