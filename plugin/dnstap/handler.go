package dnstap

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/request"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// Dnstap is the dnstap handler.
type Dnstap struct {
	Next plugin.Handler
	io   tapper
	repl replacer.Replacer

	// IncludeRawMessage will include the raw DNS message into the dnstap messages if true.
	IncludeRawMessage bool
	Identity          []byte
	Version           []byte
	ExtraFormat       string
	// enabledMessageTypes is a bitfield of enabled tap.Message_Types.
	// There's 14 message types in total, so uint64 is enough to store all of them.
	// https://github.com/dnstap/golang-dnstap/blob/ebb538e7d351a58861a8f348491828214a1d8db2/dnstap.pb.go#L216-L275
	enabledMessageTypes uint64
}

// TapMessage sends the message m to the dnstap interface, without populating "Extra" field.
func (h *Dnstap) TapMessage(m *tap.Message) {
	if h.ExtraFormat == "" {
		h.tapWithExtra(m, nil)
	} else {
		h.tapWithExtra(m, []byte(h.ExtraFormat))
	}
}

// TapMessageWithMetadata sends the message m to the dnstap interface, with "Extra" field being populated.
func (h *Dnstap) TapMessageWithMetadata(ctx context.Context, m *tap.Message, state request.Request) {
	if h.ExtraFormat == "" {
		h.tapWithExtra(m, nil)
		return
	}
	extraStr := h.repl.Replace(ctx, state, nil, h.ExtraFormat)
	h.tapWithExtra(m, []byte(extraStr))
}

func (h *Dnstap) tapWithExtra(m *tap.Message, extra []byte) {
	t := tap.Dnstap_MESSAGE
	h.io.Dnstap(&tap.Dnstap{Type: &t, Message: m, Identity: h.Identity, Version: h.Version, Extra: extra})
}

// tapClientQuery logs the client query to dnstap with the type tap.Message_CLIENT_QUERY.
func (h *Dnstap) tapClientQuery(ctx context.Context, w dns.ResponseWriter, query *dns.Msg, queryTime time.Time) {
	q := new(tap.Message)
	msg.SetQueryTime(q, queryTime)
	msg.SetQueryAddress(q, w.RemoteAddr())

	if h.IncludeRawMessage {
		buf, _ := query.Pack()
		q.QueryMessage = buf
	}
	msg.SetType(q, tap.Message_CLIENT_QUERY)
	state := request.Request{W: w, Req: query}
	h.TapMessageWithMetadata(ctx, q, state)
}

// ServeDNS logs the client query and response to dnstap and passes the dnstap Context.
func (h *Dnstap) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if h.MessageTypeEnabled(tap.Message_CLIENT_RESPONSE) {
		// Custom ResponseWriter is only used to tap CLIENT_RESPONSE messages, so we only create it if needed.
		w = &ResponseWriter{
			ResponseWriter: w,
			Dnstap:         h,
			query:          r,
			ctx:            ctx,
			queryTime:      time.Now(),
		}
	}

	// The query tap message should be sent before sending the query to the
	// forwarder. Otherwise, the tap messages will come out out of order.
	if h.MessageTypeEnabled(tap.Message_CLIENT_QUERY) {
		h.tapClientQuery(ctx, w, r, time.Now())
	}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name implements the plugin.Plugin interface.
func (h *Dnstap) Name() string { return "dnstap" }

// MessageTypeEnabled returns true if the message type t is enabled by the `message_types` configuration.
// All message types are enabled by default if the config is not provided.
func (h *Dnstap) MessageTypeEnabled(t tap.Message_Type) bool {
	return h.enabledMessageTypes&(1<<t) != 0
}
