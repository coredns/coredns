package strategy

import (
	"math/rand"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Random strategy, retrieves randomly shuffled A or AAAA records.
type Random struct {
}

// NewRandom returns new instance of Random strategy
func NewRandom() *Random {
	return &Random{}
}

// Shuffle implements Shuffler interface, retrieves randomly shuffled A or AAAA records
func (r *Random) Shuffle(_ request.Request, msg *dns.Msg) ([]dns.RR, error) {
	var shuffled []dns.RR
	var skipped []dns.RR
	for _, a := range msg.Answer {
		switch a.Header().Rrtype {
		case dns.TypeA, dns.TypeAAAA:
			shuffled = append(shuffled, a)
		default:
			skipped = append(skipped, a)
		}
	}
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return append(shuffled, skipped...), nil
}
