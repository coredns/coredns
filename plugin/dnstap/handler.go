package dnstap

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/msg"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// Dnstap is the dnstap handler.
type Dnstap struct {
	Next plugin.Handler
	io   tapper

	// IncludeRawMessage will include the raw DNS message into the dnstap messages if true.
	IncludeRawMessage bool
}

// TapMessage sends the message m to the dnstap interface.
func (h Dnstap) TapMessage(m *tap.Message) {
	t := tap.Dnstap_MESSAGE
	h.io.Dnstap(tap.Dnstap{Type: &t, Message: m})
}

func (h Dnstap) tapQuery(w dns.ResponseWriter, query *dns.Msg, queryTime time.Time) {
	q := new(tap.Message)
	msg.SetQueryTime(q, queryTime)
	msg.SetQueryAddress(q, w.RemoteAddr())

	if h.IncludeRawMessage {
		buf, _ := query.Pack()
		q.QueryMessage = buf
	}
	msg.SetType(q, tap.Message_CLIENT_QUERY)
	h.TapMessage(q)
}

// ServeDNS logs the client query and response to dnstap and passes the dnstap Context.
func (h Dnstap) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rw := &ResponseWriter{
		ResponseWriter: w,
		Dnstap:         h,
		query:          r,
		queryTime:      time.Now(),
	}

	// The query tap message should be sent before sending the query to the
	// forwarder. Otherwise, the tap messages will come out out of order.
	h.tapQuery(w, r, rw.queryTime)

	return plugin.NextOrFailure(h.Name(), h.Next, ctx, rw, r)
}

// Name implements the plugin.Plugin interface.
func (h Dnstap) Name() string { return "dnstap" }
