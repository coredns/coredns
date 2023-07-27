package dnstap

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
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
}

// TapMessage sends the message m to the dnstap interface, without interpreting the "Extra" field using metadata.
func (h Dnstap) TapMessage(m *tap.Message) {
	h.TapMessageWithMetadata(m, nil, request.Request{}, nil)
}

// TapMessageWithMetadata sends the message m to the dnstap interface, with "Extra" field being interpreted by the provided metadata context.
func (h Dnstap) TapMessageWithMetadata(m *tap.Message, ctx context.Context, state request.Request, rr *dnstest.Recorder) {
	t := tap.Dnstap_MESSAGE
	extraStr := h.ExtraFormat
	if ctx != nil && rr != nil {
		extraStr = h.repl.Replace(ctx, state, rr, extraStr)
	}
	var extra []byte
	if extraStr != "" {
		extra = []byte(extraStr)
	}
	dt := &tap.Dnstap{
		Type: &t,
		Message: m,
		Identity: h.Identity,
		Version: h.Version,
		Extra: extra,
	}
	h.io.Dnstap(dt)
}

func (h Dnstap) tapQuery(ctx context.Context, w dns.ResponseWriter, query *dns.Msg, queryTime time.Time) {
	q := new(tap.Message)
	msg.SetQueryTime(q, queryTime)
	msg.SetQueryAddress(q, w.RemoteAddr())

	if h.IncludeRawMessage {
		buf, _ := query.Pack()
		q.QueryMessage = buf
	}
	msg.SetType(q, tap.Message_CLIENT_QUERY)
	state := request.Request{W: w, Req: query}
	rrw := dnstest.NewRecorder(w)
	h.TapMessageWithMetadata(q, ctx, state, rrw)
}

// ServeDNS logs the client query and response to dnstap and passes the dnstap Context.
func (h Dnstap) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rw := &ResponseWriter{
		ResponseWriter: w,
		Dnstap:         h,
		query:          r,
		ctx:            &ctx,
		queryTime:      time.Now(),
	}

	// The query tap message should be sent before sending the query to the
	// forwarder. Otherwise, the tap messages will come out out of order.
	h.tapQuery(ctx, w, r, rw.queryTime)

	return plugin.NextOrFailure(h.Name(), h.Next, ctx, rw, r)
}

// Name implements the plugin.Plugin interface.
func (h Dnstap) Name() string { return "dnstap" }
