package refusenord

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

type handler struct {
	Next plugin.Handler
}

func (h handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if !r.RecursionDesired {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	}

	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

func (h handler) Name() string {
	return pluginName
}
