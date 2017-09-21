// Package taprw takes a query and intercepts the response.
// It will log both after the response is written.
package taprw

import (
	"fmt"

	"github.com/coredns/coredns/plugin/dnstap/msg"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/golang/protobuf/proto"
	"github.com/miekg/dns"
)

// DnstapExtra stores the extra data to be inserted into the extra field of tap.Dnstap
// Key is the tap.Dnstap message type and value is the raw byte data to be written.
type DnstapExtra struct {
	extras map[tap.Message_Type][]byte
}

// Tapper is what ResponseWriter needs to log to dnstap.
type Tapper interface {
	TapMessage(m *tap.Message, e []byte) error
	TapBuilder() msg.Builder
}

// ResponseWriter captures the client response and logs the query to dnstap.
// Single request use.
// DnsTapExtras is map of DnstapExtra.  Key is the name of the middlware that
// saves the extra data.  Value is a DnstapExtra.
type ResponseWriter struct {
	queryEpoch uint64
	Query      *dns.Msg
	dns.ResponseWriter
	Tapper
	err          error
	DnsTapExtras map[string]DnstapExtra
}

// DnstapError check if a dnstap error occurred during Write and returns it.
func (w ResponseWriter) DnstapError() error {
	return w.err
}

// QueryEpoch sets the query epoch as reported by dnstap.
func (w *ResponseWriter) QueryEpoch() {
	w.queryEpoch = msg.Epoch()
}

// extraData reads extra data saved in the DnsTapExtras map, then constructs and
// returns the extra data to be written into the tap.DnsTap message.
func (w *ResponseWriter) extraData(msgType tap.Message_Type) []byte {

	tmpMap := make(map[string][]byte)
	for k, v := range w.DnsTapExtras {
		data := v.extras[msgType]
		if data != nil || len(data) > 0 {
			tmpMap[k] = data
		}
	}
	if len(tmpMap) == 0 {
		return nil
	}

	extra, err := proto.Marshal(&msg.Extra{Extras: tmpMap})
	if err != nil {
		err = fmt.Errorf("error marshal extra data: %s", err)
		return nil
	}
	return extra
}

// WriteMsg writes back the response to the client and THEN works on logging the request
// and response to dnstap.
// Dnstap errors are to be checked by DnstapError.
func (w *ResponseWriter) WriteMsg(resp *dns.Msg) (writeErr error) {
	writeErr = w.ResponseWriter.WriteMsg(resp)
	writeEpoch := msg.Epoch()

	b := w.TapBuilder()
	b.TimeSec = w.queryEpoch
	if err := func() (err error) {
		err = b.AddrMsg(w.ResponseWriter.RemoteAddr(), w.Query)
		if err != nil {
			return
		}
		extra := w.extraData(tap.Message_CLIENT_QUERY)
		return w.TapMessage(b.ToClientQuery(), extra)

	}(); err != nil {
		w.err = fmt.Errorf("client query: %s", err)
		// don't forget to call DnstapError later
	}

	if writeErr == nil {
		if err := func() (err error) {
			b.TimeSec = writeEpoch
			if err = b.Msg(resp); err != nil {
				return
			}
			extra := w.extraData(tap.Message_CLIENT_RESPONSE)
			return w.TapMessage(b.ToClientResponse(), extra)
		}(); err != nil {
			w.err = fmt.Errorf("client response: %s", err)
		}
	}

	return
}
