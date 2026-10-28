package proxyproto

import (
	"context"
	"encoding/hex"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	cproxyproto "github.com/coredns/coredns/plugin/pkg/proxyproto"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// handler is a pass-through plugin that implements metadata.Provider to expose
// the PPv2 TLV vector captured by the listener. It is only added to the chain
// when the tlv_metadata option is set.
type handler struct {
	Next plugin.Handler
}

// Name implements plugin.Handler.
func (h handler) Name() string { return "proxyproto" }

// ServeDNS implements plugin.Handler; it only passes the request on.
func (h handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Metadata implements metadata.Provider. It publishes the raw PPv2 TLV vector
// as the "proxyproto/tlv" label, hex-encoded, when the current request carries
// TLVs. Consumers decode the hex and split it with proxyproto.SplitTLVs.
func (h handler) Metadata(ctx context.Context, state request.Request) context.Context {
	a, ok := state.W.RemoteAddr().(*cproxyproto.Addr)
	if !ok {
		return ctx
	}
	raw := a.RawTLVs()
	if len(raw) == 0 {
		return ctx
	}
	v := hex.EncodeToString(raw)
	metadata.SetValueFunc(ctx, "proxyproto/tlv", func() string { return v })
	return ctx
}

var (
	_ plugin.Handler    = handler{}
	_ metadata.Provider = handler{}
)
