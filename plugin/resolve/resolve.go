package resolve

import (
	"context"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Resolve performs CNAME target resolution on all CNAMEs in the response
type Resolve struct {
	Next    plugin.Handler
	Zones   []string
	DoCNAME bool
	DoSRV   bool
}

// Name implements the Handler interface.
func (c Resolve) Name() string { return name() }
func name() string             { return "resolve" }

// ServeDNS implements the plugin.Handle interface.
func (c Resolve) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r, Context: ctx}

	zone := plugin.Zones(c.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	if state.QType() == dns.TypeCNAME {
		return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
	}

	// Run the query through the rest of the plugin chain using a non-writer
	nw := nonwriter.New(w)
	rcode, err := plugin.NextOrFailure(c.Name(), c.Next, ctx, nw, r)
	if err != nil || !plugin.ClientWrite(rcode) {
		return rcode, err
	}

	// Look at each answer and do lookups for any CNAME/SRV answers
	for i := 0; i < len(nw.Msg.Answer); i++ {
		a := nw.Msg.Answer[i]
		if c.DoCNAME && a.Header().Rrtype == dns.TypeCNAME {
			targetAnswer := c.queryTarget(state, a.(*dns.CNAME).Target, state.QType())
			nw.Msg.Answer = addTarget(nw.Msg.Answer, targetAnswer)
		}
		if c.DoSRV && a.Header().Rrtype == dns.TypeSRV {
			targetAnswerA := c.queryTarget(state, a.(*dns.SRV).Target, dns.TypeA)
			nw.Msg.Extra = addTarget(nw.Msg.Extra, targetAnswerA)

			targetAnswerAAAA := c.queryTarget(state, a.(*dns.SRV).Target, dns.TypeAAAA)
			nw.Msg.Extra = addTarget(nw.Msg.Extra, targetAnswerAAAA)
		}
	}

	// Write the response to the client
	w.WriteMsg(nw.Msg)
	return rcode, err
}

// queryTarget looks up records for the qname by querying against the plugin chain, using a non-writer
func (c Resolve) queryTarget(state request.Request, qName string, qType uint16) []dns.RR {
	target := nonwriter.New(state.W)
	r := state.Req.Copy()
	r.SetQuestion(qName, qType)
	rcode, err := plugin.NextOrFailure(c.Name(), c.Next, state.Context, target, r)
	if err != nil || target.Msg == nil || !plugin.ClientWrite(rcode) {
		return nil
	}
	return target.Msg.Answer
}

// addTarget adds the answers from 'target' to the answers in 'clientResponse' ensuring that targets are not already in the
// client response (not creating duplicates)
func addTarget(clientRR, targetRR []dns.RR) []dns.RR {
	for _, t := range targetRR {
		unique := true
		for _, b := range clientRR {
			if rrDiff(t, b) {
				continue
			}
			unique = false
			break
		}
		if unique {
			clientRR = append(clientRR, t)
		}
	}
	return clientRR
}

// rrDiff returns true if the two dns.RR are different in name, type, or target
func rrDiff(a, b dns.RR) bool {
	if a.Header().Name != b.Header().Name {
		return true
	}
	if a.Header().Rrtype != b.Header().Rrtype {
		return true
	}
	if a.Header().Rrtype == dns.TypeA && !a.(*dns.A).A.Equal(b.(*dns.A).A) {
		return true
	}
	if a.Header().Rrtype == dns.TypeAAAA && !a.(*dns.AAAA).AAAA.Equal(b.(*dns.AAAA).AAAA) {
		return true
	}
	if a.Header().Rrtype == dns.TypeCNAME && a.(*dns.CNAME).Target != b.(*dns.CNAME).Target {
		return true
	}
	if a.Header().Rrtype == dns.TypeSRV && a.(*dns.SRV).Target != b.(*dns.SRV).Target {
		return true
	}
	// All other record types
	if strings.TrimPrefix(a.String(), a.Header().String()) != strings.TrimPrefix(b.String(), b.Header().String()) {
		return true
	}
	return false
}
