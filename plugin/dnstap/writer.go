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

	tm := new(tap.Message)
	msg.SetResponseTime(tm, time.Now())
	msg.SetQueryTime(tm, w.QueryTime)
	if err := msg.SetQueryAddress(tm, w.RemoteAddr()); err != nil {
		w.Err = err
		return err
	}

	if w.IncludeRawMessage {
		buf, err := w.Query.Pack()
		if err != nil {
			w.Err = err
			return err
		}
		tm.QueryMessage = buf
	}
	msg.SetType(tm, tap.Message_CLIENT_QUERY)
	w.TapMessage(tm)

	if writeErr != nil {
		return writeErr
	}

	if w.IncludeRawMessage {
		buf, err := resp.Pack()
		if err != nil {
			w.Err = err
			return err
		}
		tm.ResponseMessage = buf
	}
	tm.QueryMessage = nil // zero this, to not send it again
	msg.SetType(tm, tap.Message_CLIENT_RESPONSE)
	w.TapMessage(tm)
	return writeErr
}
