package dnstap

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/dnstapio"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// Dnstap is the dnstap handler.
type Dnstap struct {
	Next plugin.Handler
	io   dnstapio.Tapper

	// IncludeRawMessage will include the raw DNS message into the dnstap messages if true.
	IncludeRawMessage bool
}

// TapMessage sends the message m to the dnstap interface.
func (h Dnstap) TapMessage(m *tap.Message) {
	t := tap.Dnstap_MESSAGE
	h.io.Dnstap(tap.Dnstap{Type: &t, Message: m})
}

// ServeDNS logs the client query and response to dnstap and passes the dnstap Context.
func (h Dnstap) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rw := &ResponseWriter{
		ResponseWriter: w,
		Dnstap:         h,
		Query:          r,
		QueryTime:      time.Now(),
	}

	return plugin.NextOrFailure(h.Name(), h.Next, ctx, rw, r)
}

// Name implements the plugin.Plugin interface.
func (h Dnstap) Name() string { return "dnstap" }
