// Package fallback implements a fallback plugin for CoreDNS
package fallback

import (
	"context"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/proxy"
)

// Fallback plugin
type Fallback struct {
	Next  plugin.Handler
	trace plugin.Handler
	rules map[int]proxy.Upstream
	proxy proxyCreator
}

// proxyCreator creates a proxy with the specified upstream
type proxyCreator interface {
	Create(trace plugin.Handler, upstream proxy.Upstream) plugin.Handler
}

// fallbackProxyCreator implements the proxyCreator interface
// Used by the fallback plugin to create proxy using specified for upstream
type fallbackProxyCreator struct{}

func (f fallbackProxyCreator) Create(trace plugin.Handler, upstream proxy.Upstream) plugin.Handler {
	return &proxy.Proxy{Trace: trace, Upstreams: &[]proxy.Upstream{upstream}}
}

func newFallback(trace plugin.Handler) (f *Fallback) {
	return &Fallback{trace: trace, rules: make(map[int]proxy.Upstream), proxy: fallbackProxyCreator{}}
}

// ServeDNS implements the plugin.Handler interface.
func (f Fallback) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	nw := nonwriter.New(w)
	rcode, err := plugin.NextOrFailure(f.Name(), f.Next, ctx, nw, r)
	if err != nil {
		return rcode, err
	}
	//WTF? WHY DOES PROXY ALWAYS RETURN 0
	if rcode == 0 {
		rcode = nw.Msg.Rcode
	}
	if u, ok := f.rules[rcode]; ok {
		p := f.proxy.Create(f.trace, u)
		return p.ServeDNS(ctx, w, r)
	}
	w.WriteMsg(nw.Msg)
	return rcode, nil
}

// Name implements the Handler interface.
func (f Fallback) Name() string { return "fallback" }
