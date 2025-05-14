package quic

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

// QUIC represents a plugin instance that can proxy requests to another (DNS) server via
// DNS-over-QUIC (DoQ).
type QUIC struct {
	proxies []*Proxy
	p       Policy

	from    string
	ignored []string

	tlsConfig     *tls.Config
	tlsServerName string

	Next plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (q *QUIC) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if !q.match(state) {
		return plugin.NextOrFailure(q.Name(), q.Next, ctx, w, r)
	}

	var (
		span, child ot.Span
		ret         *dns.Msg
		err         error
		i           int
	)
	span = ot.SpanFromContext(ctx)
	list := q.list()
	deadline := time.Now().Add(defaultTimeout)

	for time.Now().Before(deadline) {
		if i >= len(list) {
			// reached the end of list without any answer
			if ret != nil {
				return dns.RcodeServerFailure, ErrNoAnswer
			}
			break
		}

		proxy := list[i]
		i++

		if span != nil {
			child = span.Tracer().StartSpan("query", ot.ChildOf(span.Context()))
			ctx = ot.ContextWithSpan(ctx, child)
		}

		ret, err = proxy.query(ctx, r)
		if err != nil {
			// Continue with the next proxy
			continue
		}

		if child != nil {
			child.Finish()
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(ret) {
			debug.Hexdumpf(ret, "Wrong reply for id: %d, %s %d", ret.Id, state.QName(), state.QType())

			formerr := new(dns.Msg)
			formerr.SetRcode(state.Req, dns.RcodeFormatError)
			w.WriteMsg(formerr)
			return 0, nil
		}

		w.WriteMsg(ret)
		return 0, nil
	}

	// SERVFAIL if all healthy proxys returned errors.
	if err != nil {
		// just return the last error received
		return dns.RcodeServerFailure, err
	}

	return dns.RcodeServerFailure, ErrNoHealthy
}

// NewQUIC returns a new QUIC.
func NewQUIC() *QUIC {
	return &QUIC{
		p: new(random),
	}
}

// Name implements the Handler interface.
func (q *QUIC) Name() string { return "quic" }

// Len returns the number of configured proxies.
func (q *QUIC) len() int { return len(q.proxies) }

func (q *QUIC) match(state request.Request) bool {
	if !plugin.Name(q.from).Matches(state.Name()) || !q.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (q *QUIC) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(q.from) {
		return true
	}

	for _, ignore := range q.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// List returns a set of proxies to be used for this client depending on the policy in p.
func (q *QUIC) list() []*Proxy { return q.p.List(q.proxies) }

const defaultTimeout = 5 * time.Second

var (
	// ErrNoHealthy means no healthy proxies left.
	ErrNoHealthy = errors.New("no healthy QUIC proxies")

	// ErrNoAnswer means no proxy responded
	ErrNoAnswer = errors.New("no answer from QUIC proxies")
)
