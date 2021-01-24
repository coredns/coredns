package minimal

import (
	"context"
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
	w.WriteMsg(m.minimizeResponse(nw.Msg))
	return rcode, err
}

func (m *minimalHandler) minimizeResponse(msg *dns.Msg) *dns.Msg {
	if ty, _ := response.Typify(msg, time.Now().UTC()); ty == response.NoError {
		msg.Extra = nil
		msg.Ns = nil
	}
	return msg
}
