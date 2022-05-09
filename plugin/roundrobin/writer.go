package roundrobin

import (
	"fmt"

	"github.com/coredns/coredns/plugin/roundrobin/internal/strategy"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// MessageWriter is a response writer that shuffles A or AAAA records
type MessageWriter struct {
	dns.ResponseWriter
	strategy strategy.Shuffler
	state    request.Request
}

// NewMessageWriter provides new MessageWriter instance
func NewMessageWriter(w dns.ResponseWriter, msg *dns.Msg, strategy strategy.Shuffler) (*MessageWriter, error) {
	return &MessageWriter{
		state:          request.Request{W: w, Req: msg},
		ResponseWriter: w,
		strategy:       strategy,
	}, nil
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *MessageWriter) WriteMsg(msg *dns.Msg) error {
	if msg == nil || len(msg.Question) == 0 {
		return fmt.Errorf("writing mesage (nil)")
	}
	if msg.Rcode != dns.RcodeSuccess {
		return r.ResponseWriter.WriteMsg(msg)
	}
	q := msg.Question[0]

	if q.Qtype == dns.TypeAXFR || q.Qtype == dns.TypeIXFR {
		return r.ResponseWriter.WriteMsg(msg)
	}

	if answer, err := r.strategy.Shuffle(r.state, msg); err == nil {
		msg.Answer = answer
	} else {
		log.Errorf("RoundRobin plugin failed %s.", err)
	}

	return r.ResponseWriter.WriteMsg(msg)
}

// Write implements the dns.ResponseWriter interface.
func (r *MessageWriter) Write(buf []byte) (int, error) {
	// Should we pack and unpack here to fiddle with the packet... Not likely.
	log.Warning("RoundRobin called with Write: not shuffling records")
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}
