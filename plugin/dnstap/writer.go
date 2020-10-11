package dnstap

import (
	"time"

	"github.com/coredns/coredns/plugin/dnstap/msg"
	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// ResponseWriter captures the client response and logs the query to dnstap.
// Single request use.
type ResponseWriter struct {
	QueryTime time.Time
	Query     *dns.Msg
	dns.ResponseWriter

	Dnstap
	Err error
}

// WriteMsg writes back the response to the client and THEN works on logging the request
// and response to dnstap.
func (w *ResponseWriter) WriteMsg(resp *dns.Msg) error {
	writeErr := w.ResponseWriter.WriteMsg(resp)

	q := new(tap.Message)
	msg.SetQueryTime(q, w.QueryTime)
	msg.SetQueryAddress(q, w.RemoteAddr())

	if w.IncludeRawMessage {
		buf, err := w.Query.Pack()
		if err != nil {
			w.Err = err
			return err
		}
		q.QueryMessage = buf
	}
	msg.SetType(q, tap.Message_CLIENT_QUERY)
	w.TapMessage(q)

	if writeErr != nil {
		return writeErr
	}

	r := new(tap.Message)
	msg.SetQueryTime(r, w.QueryTime)
	msg.SetResponseTime(r, time.Now())
	msg.SetQueryAddress(r, w.RemoteAddr())

	if w.IncludeRawMessage {
		buf, err := resp.Pack()
		if err != nil {
			w.Err = err
			return err
		}
		r.ResponseMessage = buf
	}

	msg.SetType(r, tap.Message_CLIENT_RESPONSE)
	w.TapMessage(r)
	return nil
}
