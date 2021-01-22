package minimal_responses

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/miekg/dns"
)

// minimalResponse implements the plugin.Handler interface.
type minimalResponse struct {
	Next plugin.Handler
}

func (m *minimalResponse) Name() string { return "minimal-responses" }

func (m *minimalResponse) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// if plugin active, try minimizing response
	nw := nonwriter.New(w)

	// Call all the next plugin in the chain.
	rcode, err := plugin.NextOrFailure(m.Name(), m.Next, ctx, nw, r)
	if err != nil {
		// if error received then just return the error
		return rcode, err
	}
	// else write minimized response
	w.WriteMsg(m.minimizeResponse(nw.Msg))
	return rcode, err
}

func (m *minimalResponse) minimizeResponse(msg *dns.Msg) *dns.Msg {
	if ty, _ := response.Typify(msg, time.Now().UTC()); ty == response.NoError {
		msg.Extra = nil
		msg.Ns = nil
	}
	return msg
}
