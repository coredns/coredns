package strategy

import (
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Shuffler implementation returns mixed (or rotated) A or AAAA result.
type Shuffler interface {
	// Shuffle runs round-robin algorithm. Each call to Shuffle causes
	// the A or AAAA record positions in the response to change.
	Shuffle(req request.Request, msg *dns.Msg) ([]dns.RR, error)
}
