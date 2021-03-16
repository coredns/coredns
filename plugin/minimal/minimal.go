package minimal

import (
	"context"
	"fmt"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/response"

	"github.com/miekg/dns"
)

// minimalHandler implements the plugin.Handler interface.
type minimalHandler struct {
	Next plugin.Handler
}

func (m *minimalHandler) Name() string { return "minimal" }

func (m *minimalHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	nw := nonwriter.New(w)

	rcode, err := plugin.NextOrFailure(m.Name(), m.Next, ctx, nw, r)
	if err != nil {
		return rcode, err
	}

	ty, _ := response.Typify(nw.Msg, time.Now().UTC())
	cl := response.Classify(ty)

	// if response is Denial or Error pass through also if the type is Delegation pass through
	if cl == response.Denial || cl == response.Error || ty == response.Delegation {
		w.WriteMsg(nw.Msg)
		return 0, nil
	}
	if ty != response.NoError {
		w.WriteMsg(nw.Msg)
		return 0, plugin.Error("minimal", fmt.Errorf("unhandled response type %q for %q", ty, nw.Msg.Question[0].Name))
	}

	// copy over the original Msg params, deep copy not required as RRs are not modified
	d := &dns.Msg{
		MsgHdr:   nw.Msg.MsgHdr,
		Compress: nw.Msg.Compress,
		Question: nw.Msg.Question,
		Answer:   nw.Msg.Answer,
		Ns:       nil,
		Extra:    nil,
	}

	w.WriteMsg(d)
	return 0, nil
}
