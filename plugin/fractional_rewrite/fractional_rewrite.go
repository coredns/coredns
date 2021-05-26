package fractional_rewrite

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type fractionalRewrite struct {
	Next plugin.Handler
	Rule Rule
}

func (fr fractionalRewrite) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	fr.Rule.Rewrite(ctx, state)
	return plugin.NextOrFailure(fr.Name(), fr.Next, ctx, w, r)
}

func (fr fractionalRewrite) Name() string { return "frational_rewrite" }
