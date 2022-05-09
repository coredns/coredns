package roundrobin

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/roundrobin/internal/strategy"

	"github.com/miekg/dns"
)

// RoundRobin provider
type RoundRobin struct {
	Next     plugin.Handler
	strategy strategy.Shuffler
}

const (
	strategyWeight    = "weight"
	strategyStateless = "stateless"
	strategyRandom    = "random"
	strategyStateful  = "stateful"
)

// New create new RoundRobin instance
func New(next plugin.Handler, strategy strategy.Shuffler) *RoundRobin {
	return &RoundRobin{
		Next:     next,
		strategy: strategy,
	}
}

// ServeDNS makes the middleware accessible by implementing Handler interface
func (rr *RoundRobin) ServeDNS(ctx context.Context, w dns.ResponseWriter, msg *dns.Msg) (int, error) {
	wrr, err := NewMessageWriter(w, msg, rr.strategy)
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	return plugin.NextOrFailure(rr.Name(), rr.Next, ctx, wrr, msg)
}

// Name retrieves plugin name, implements Handler interface
func (rr *RoundRobin) Name() string {
	return pluginName
}
