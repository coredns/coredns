// Package nonwriter implements a dns.ResponseWriter that never writes, but captures the dns.Msg being written.
package nonwriter

import (
	"github.com/miekg/dns"
)

// Writer is a type of ResponseWriter that captures the message, but never writes to the client.
//
// A Writer stands in for a single response. A handler that answers with a stream of messages, as
// plugin/transfer does for AXFR and IXFR, must not be given one: Msg holds only the last message
// written. When more than one message is written Msgs holds all of them in order, so a caller
// that may see a stream can detect and forward it. It stays nil for a single write.
type Writer struct {
	dns.ResponseWriter
	Msg  *dns.Msg
	Msgs []*dns.Msg
}

// New makes and returns a new NonWriter.
func New(w dns.ResponseWriter) *Writer { return &Writer{ResponseWriter: w} }

// WriteMsg records the message, but doesn't write it itself.
func (w *Writer) WriteMsg(res *dns.Msg) error {
	if w.Msg != nil {
		if w.Msgs == nil {
			w.Msgs = append(w.Msgs, w.Msg)
		}
		w.Msgs = append(w.Msgs, res)
	}
	w.Msg = res
	return nil
}
