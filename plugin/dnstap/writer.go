package dnstap

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/request"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// ResponseWriter captures the client response and logs the query to dnstap.
type ResponseWriter struct {
	queryTime time.Time
	query     *dns.Msg
	ctx       context.Context
	written   bool // whether WriteMsg was called, i.e. a response was written to the client
	dns.ResponseWriter
	*Dnstap
}

// WriteMsg writes back the response to the client and THEN works on logging the request and response to dnstap.
func (w *ResponseWriter) WriteMsg(resp *dns.Msg) error {
	err := w.ResponseWriter.WriteMsg(resp)
	if err != nil {
		return err
	}
	w.written = true
	w.tapResponse(resp)
	return nil
}

// tapResponse sends a CLIENT_RESPONSE dnstap message for resp. It does not
// write anything back to the client; the caller is responsible for that.
func (w *ResponseWriter) tapResponse(resp *dns.Msg) {
	r := new(tap.Message)
	msg.SetQueryTime(r, w.queryTime)
	msg.SetResponseTime(r, time.Now())
	msg.SetQueryAddress(r, w.RemoteAddr())

	if w.IncludeRawMessage {
		buf, _ := resp.Pack()
		r.ResponseMessage = buf
	}

	msg.SetType(r, tap.Message_CLIENT_RESPONSE)
	state := request.Request{W: w.ResponseWriter, Req: w.query}
	w.TapMessageWithMetadata(w.ctx, r, state)
}
