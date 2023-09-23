// Package oneaddr is a plugin for rewriting responses to retain only single address
package oneaddr

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// OneAddr is a plugin to rewrite responses to retain only first address in the response.
type OneAddr struct {
	Next plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (oa OneAddr) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rw := &OneAddrResponseWriter{ResponseWriter: w}
	return plugin.NextOrFailure(oa.Name(), oa.Next, ctx, rw, r)
}

// Name implements the Handler interface.
func (oa OneAddr) Name() string { return "oneaddr" }
