package strategy

import (
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Stateless strategy, retrieves rotated A or AAAA records.
// It doesn't remember the previous state, so the previous state
// must be provided by client request in Extra section.
// Read README.md for more details.
type Stateless struct {
}

// NewStateless provides new Stateless instance
func NewStateless() *Stateless {
	return &Stateless{}
}

// Shuffle implements Shuffler interface. Rotates A or AAAA consistently by one position.
// The records from previous state must be provided by client otherwise it copy
// response from *dns.Msg response and rotates it by one position.
func (r *Stateless) Shuffle(req request.Request, msg *dns.Msg) ([]dns.RR, error) {
	return newStateless(req.Req, msg).updateState().rotate().getAnswers()
}
