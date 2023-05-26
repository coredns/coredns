// Package loadbalance shuffles A, AAAA and MX records.
package loadbalance

import (
	"github.com/miekg/dns"
)

const (
	ramdomShufflePolicy      = "round_robin"
	weightedRoundRobinPolicy = "weighted"
)

// LoadBalanceResponseWriter is a response writer that shuffles A, AAAA and MX records.
type LoadBalanceResponseWriter struct {
	dns.ResponseWriter
	shuffle func(*dns.Msg) *dns.Msg
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *LoadBalanceResponseWriter) WriteMsg(res *dns.Msg) error {
	if res.Rcode != dns.RcodeSuccess {
		return r.ResponseWriter.WriteMsg(res)
	}

	if res.Question[0].Qtype == dns.TypeAXFR || res.Question[0].Qtype == dns.TypeIXFR {
		return r.ResponseWriter.WriteMsg(res)
	}

	return r.ResponseWriter.WriteMsg(r.shuffle(res))
}

func randomShuffle(res *dns.Msg) *dns.Msg {
	res.Answer = roundRobin(res.Answer)
	res.Ns = roundRobin(res.Ns)
	res.Extra = roundRobin(res.Extra)
	return res
}

func roundRobin(in []dns.RR) []dns.RR {
	cname := []dns.RR{}
	address := []dns.RR{}
	mx := []dns.RR{}
	rest := []dns.RR{}
	for _, r := range in {
		switch r.Header().Rrtype {
		case dns.TypeCNAME:
			cname = append(cname, r)
		case dns.TypeA, dns.TypeAAAA:
			address = append(address, r)
		case dns.TypeMX:
			mx = append(mx, r)
		default:
			rest = append(rest, r)
		}
	}

	roundRobinShuffle(address)
	roundRobinShuffle(mx)

	out := append(cname, rest...)
	out = append(out, address...)
	out = append(out, mx...)
	return out
}

func roundRobinShuffle(records []dns.RR) {
	switch l := len(records); l {
	case 0, 1:
		break
	case 2:
		if dns.Id()%2 == 0 {
			records[0], records[1] = records[1], records[0]
		}
	default:
		for j := 0; j < l; j++ {
			p := j + (int(dns.Id()) % (l - j))
			if j == p {
				continue
			}
			records[j], records[p] = records[p], records[j]
		}
	}
}

// Write implements the dns.ResponseWriter interface.
func (r *LoadBalanceResponseWriter) Write(buf []byte) (int, error) {
	// Should we pack and unpack here to fiddle with the packet... Not likely.
	log.Warning("LoadBalance called with Write: not shuffling records")
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}
