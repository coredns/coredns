package header

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// Header modifies dns.MsgHdr in the responses
type Header struct {
	Rules []Rule
	Next  plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (h Header) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	wr := ResponseHeaderWriter{ResponseWriter: w, Rules: h.Rules}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, &wr, r)
}

// Name implements the plugin.Handler interface.
func (h Header) Name() string { return "header" }
