// Package forward implements a forwarding proxy. It caches an upstream net.Conn for some time, so if the same
// client returns the upstream's Conn will be precached. Depending on how you benchmark this looks to be
// 50% faster than just opening a new connection for every client. It works with UDP and TCP and uses
// inband healthchecking.
package forward

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/forward/proxy"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
)

var log = clog.NewWithPlugin("forward")

// Forward represents a plugin instance that can proxy requests to another (DNS) server. It has a list
// of proxies each representing one upstream proxy.
type Forward struct {
	proxies    []proxy.Proxy
	p          Policy
	hcInterval time.Duration

	from    string
	ignored []string

	tlsConfig     map[string]*tls.Config
	tlsServerName map[string]string
	maxfails      uint32
	expire        time.Duration

	opts options // also here for testing

	m    *metrics.Metrics
	Next plugin.Handler
}

// New returns a new Forward.
func New(metrics *metrics.Metrics) *Forward {
	f := &Forward{
		maxfails:      2,
		expire:        defaultExpire,
		p:             new(random),
		from:          ".",
		hcInterval:    hcInterval,
		tlsConfig:     map[string]*tls.Config{},
		tlsServerName: map[string]string{},
		m:             metrics,
	}
	return f
}

// SetProxy appends p to the proxy list and starts healthchecking.
func (f *Forward) SetProxy(p proxy.Proxy) {
	f.proxies = append(f.proxies, p)
	p.Start(f.hcInterval)
}

// Len returns the number of configured proxies.
func (f *Forward) Len() int { return len(f.proxies) }

// Name implements plugin.Handler.
func (f *Forward) Name() string { return "forward" }

// ServeDNS implements plugin.Handler.
func (f *Forward) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if !f.match(state) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	var span, child ot.Span
	span = ot.SpanFromContext(ctx)
	i := 0
	fails := 0
	var ret *dns.Msg
	var err error

	list := f.List()
	deadline := time.Now().Add(defaultTimeout)

	for time.Now().Before(deadline) {
		if i >= len(list) {
			// reached the end of list without any answer
			if ret != nil {
				// write empty response and finish
				w.WriteMsg(ret)
			}
			break
		}

		proxy := list[i]
		i++

		if proxy.Down(f.maxfails) {
			fails++
			if fails < len(f.proxies) {
				// Continue with the rest of upstream proxies
				continue
			}
			// All upstream proxies are dead
			return dns.RcodeServerFailure, ErrNoHealthy
		}

		if span != nil {
			child = span.Tracer().StartSpan("connect", ot.ChildOf(span.Context()))
			ctx = ot.ContextWithSpan(ctx, child)
		}

		ret, err = proxy.Query(ctx, state)

		if child != nil {
			child.Finish()
		}

		if err != nil {
			// Kick off health check to see if *our* upstream is broken.
			if f.maxfails != 0 {
				proxy.Healthcheck()
			}
			continue
		}

		if ret.Rcode == dns.RcodeNameError {
			// Continue if no answer found
			continue
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(ret) {
			debug.Hexdumpf(ret, "Wrong reply for id: %d, %s/%d", state.QName(), state.QType())

			formerr := state.ErrorMessage(dns.RcodeFormatError)
			w.WriteMsg(formerr)
			return 0, nil
		}

		w.WriteMsg(ret)

		break
	}
	return 0, nil
}

func (f *Forward) match(state request.Request) bool {
	if !plugin.Name(f.from).Matches(state.Name()) || !f.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (f *Forward) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(f.from) {
		return true
	}

	for _, ignore := range f.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// List returns a set of proxies to be used for this client depending on the policy in f.
func (f *Forward) List() []proxy.Proxy { return f.p.List(f.proxies) }

var (
	// ErrNoHealthy means no healthy proxies left.
	ErrNoHealthy = errors.New("no healthy proxies")
)

// options holds various options that can be set.
type options struct {
	forceTCP  bool
	preferUDP bool
}

const defaultTimeout = 5 * time.Second

const (
	hcInterval    = 500 * time.Millisecond
	defaultExpire = 10 * time.Second
)
