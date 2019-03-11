package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
)

// GRPC is an grpc plugin to show how to write a plugin.
type GRPC struct {
	proxies    []*Proxy
	p          Policy
	hcInterval time.Duration

	from    string
	ignored []string

	tlsConfig     *tls.Config
	tlsServerName string
	maxfails      uint32

	Next plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (g *GRPC) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if !g.match(state) {
		return plugin.NextOrFailure(g.Name(), g.Next, ctx, w, r)
	}

	var (
		span, child ot.Span
		ret         *dns.Msg
		err         error
		fails       int
		i           int
	)
	span = ot.SpanFromContext(ctx)
	list := g.list()
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

		if proxy.down(g.maxfails) {
			fails++
			if fails < len(g.proxies) {
				continue
			}
			// All upstream proxies are dead
			return dns.RcodeServerFailure, ErrNoHealthy
		}

		if span != nil {
			child = span.Tracer().StartSpan("query", ot.ChildOf(span.Context()))
			ctx = ot.ContextWithSpan(ctx, child)
		}

		ret, err = proxy.query(ctx, r)
		if err != nil {
			// Kick off health check to see if *our* upstream is broken.
			if g.maxfails != 0 {
				proxy.healthcheck()
			}
			continue
		}

		if child != nil {
			child.Finish()
		}

		// Continue if no answer has been found
		if ret.Rcode == dns.RcodeNameError {
			continue
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(ret) {
			debug.Hexdumpf(ret, "Wrong reply for id: %d, %s %d", ret.Id, state.QName(), state.QType())

			formerr := state.ErrorMessage(dns.RcodeFormatError)
			w.WriteMsg(formerr)
			return 0, nil
		}

		w.WriteMsg(ret)
		return 0, nil
	}

	return 0, nil
}

// NewGRPC returns a new GRPC.
func newGRPC() *GRPC {
	g := &GRPC{
		p:          new(random),
		maxfails:   2,
		hcInterval: hcInterval,
	}
	return g
}

// Name implements the Handler interface.
func (g *GRPC) Name() string { return "grpc" }

// Len returns the number of configured proxies.
func (g *GRPC) len() int { return len(g.proxies) }

func (g *GRPC) match(state request.Request) bool {
	if !plugin.Name(g.from).Matches(state.Name()) || !g.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (g *GRPC) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(g.from) {
		return true
	}

	for _, ignore := range g.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// List returns a set of proxies to be used for this client depending on the policy in p.
func (g *GRPC) list() []*Proxy { return g.p.List(g.proxies) }

// OnStartup starts a goroutines for all proxies.
func (g *GRPC) onStartup() (err error) {
	for _, p := range g.proxies {
		p.start(g.hcInterval)
	}
	return nil
}

// OnShutdown stops all configured proxies.
func (g *GRPC) onShutdown() error {
	for _, p := range g.proxies {
		p.stop()
	}
	return nil
}

const (
	hcInterval     = 500 * time.Millisecond
	defaultTimeout = 5 * time.Second
)

var (
	// ErrNoHealthy means no healthy proxies left.
	ErrNoHealthy = errors.New("no healthy proxies")
)
