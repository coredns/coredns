package transfer

import (
	"fmt"
	"net"

	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"

	"github.com/mholt/caddy"
)

// Parse parses transfer statements: 'transfer to [address...]'.
func Parse(c *caddy.Controller, secondary bool) (tos, froms []string, err error) {
	if !c.NextArg() {
		return nil, nil, c.ArgErr()
	}
	value := c.Val()
	switch value {
	case "to":
		tos = c.RemainingArgs()
		for i := range tos {
			if tos[i] != "*" {
				normalized, err := dnsutil.ParseHostPort(tos[i], "53")
				if err != nil {
					return nil, nil, err
				}
				tos[i] = normalized
			}
		}

	case "from":
		if !secondary {
			return nil, nil, fmt.Errorf("can't use `transfer from` when not being a secondary")
		}
		froms = c.RemainingArgs()
		for i := range froms {
			if froms[i] != "*" {
				normalized, err := dnsutil.ParseHostPort(froms[i], "53")
				if err != nil {
					return nil, nil, err
				}
				froms[i] = normalized
			} else {
				return nil, nil, fmt.Errorf("can't use '*' in transfer from")
			}
		}
	}
	return
}

// Allowed checks if incoming request for transferring the zone is allowed according to the ACLs.
func Allowed(state request.Request, transferTo []string) bool {
	for _, t := range transferTo {
		if t == "*" {
			return true
		}
		// If remote IP matches we accept.
		remote := state.IP()
		to, _, err := net.SplitHostPort(t)
		if err != nil {
			continue
		}
		if to == remote {
			return true
		}
	}
	return false
}

// Out starts a transfer to the remote server (out from CoreDNS). RRs must start and end with a SOA record, this
// is not enforced.
func Out(state request.Request, rrs []dns.RR) {
	ch := make(chan *dns.Envelope)
	defer close(ch)

	tr := new(dns.Transfer)
	go tr.Out(state.W, state.Req, ch)

	j, l := 0, 0
	for i, r := range rrs {
		l += dns.Len(r)
		if l > envelopeSize {
			ch <- &dns.Envelope{RR: rrs[j:i]}
			l = 0
			j = i
		}
	}
	if j < len(rrs) {
		ch <- &dns.Envelope{RR: rrs[j:]}
	}

	state.W.Hijack()
	// state.W.Close() // Client closes connection
}

const envelopeSize = 32000 // Start a new envelop after message reaches this size in bytes.
