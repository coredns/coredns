package cnameresolver

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// CNAMEResolve performs CNAME target resolution on all CNAMEs in the response
type CNAMEResolve struct {
	Next  plugin.Handler
	Zones []string
}

// ServeDNS implements the plugin.Handle interface.
func (c *CNAMEResolve) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	if state.QType() != dns.TypeA && state.QType() != dns.TypeAAAA {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	zone := plugin.Zones(c.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	nw := nonwriter.New(w)

	rcode, err := plugin.NextOrFailure(c.Name(), c.Next, ctx, nw, r)

	if err != nil {
		return rcode, err
	}

	u, _ := upstream.New(nil)

	for i := 0; i < len(nw.Msg.Answer); i++ {
		a := nw.Msg.Answer[i]
		unique := true
		if a.Header().Rrtype != dns.TypeCNAME {
			continue
		}

		m, err := u.Lookup(state, a.(*dns.CNAME).Target, state.QType())
		if err != nil {
			continue
		}
		for _, t := range m.Answer {
			for _, b := range nw.Msg.Answer {
				if t.Header().Name != b.Header().Name {
					continue
				}
				if t.Header().Rrtype != b.Header().Rrtype {
					continue
				}
				if t.Header().Rrtype == dns.TypeCNAME && t.(*dns.CNAME).Target != b.(*dns.CNAME).Target {
					continue
				}
				if t.Header().Rrtype == dns.TypeA && !t.(*dns.A).A.Equal(b.(*dns.A).A) {
					continue
				}
				if t.Header().Rrtype == dns.TypeAAAA && !t.(*dns.AAAA).AAAA.Equal(b.(*dns.AAAA).AAAA) {
					continue
				}
				unique = false
				break
			}
			if unique {
				nw.Msg.Answer = append(nw.Msg.Answer, t)
			}
		}
	}

	if plugin.ClientWrite(rcode) {
		nw.WriteMsg(r)
	}
	return rcode, err
}

// Name implements the Handler interface.
func (c *CNAMEResolve) Name() string { return "cnameresolver" }
