package header

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// Header modifies flags of dns.MsgHdr in queries and / or responses
type Header struct {
	QueryRules    []Rule
	ResponseRules []Rule
	Next          plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (h Header) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	applyRules(r, h.QueryRules)

	wr := ResponseHeaderWriter{ResponseWriter: w, Rules: h.ResponseRules}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, &wr, r)
}

// Name implements the plugin.Handler interface.
func (h Header) Name() string { return "header" }
