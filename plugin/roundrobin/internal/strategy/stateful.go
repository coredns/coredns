package strategy

import (
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Stateful strategy, retrieves rotated A or AAAA records.
// It remembers the previous state, so it is able to rotate
// A or AAAA consistently by one position depending on the
// subnet+question+dnsType call
type Stateful struct {
	state *stateful
}

// NewStateful creates new instance of Stateful
func NewStateful() *Stateful {
	return &Stateful{
		state: newStateful(),
	}
}

// Shuffle implements Shuffler interface. Rotates A or AAAA
// consistently by one position
func (s *Stateful) Shuffle(req request.Request, res *dns.Msg) ([]dns.RR, error) {
	return s.state.update(&req, res)
}
