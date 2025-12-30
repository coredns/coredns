// Package loadbalance is a plugin for rewriting responses to do "load balancing".
package loadbalance

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// LoadBalance is a plugin to rewrite responses for "load balancing".
type LoadBalance struct {
	Next    plugin.Handler
	shuffle func(*dns.Msg) *dns.Msg
}

// ServeDNS implements the plugin.Handler interface.
func (lb LoadBalance) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rw := &LoadBalanceResponseWriter{ResponseWriter: w, shuffle: lb.shuffle}
	return plugin.NextOrFailure(lb.Name(), lb.Next, ctx, rw, r)
}

// Name implements the Handler interface.
func (lb LoadBalance) Name() string { return "loadbalance" }
