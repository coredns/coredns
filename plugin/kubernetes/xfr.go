package kubernetes

import (
	"context"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// TransferDeprecated performs a zone transfer. This function is deprecated,
// zone transfer is replaced by the transfer plugin. It should be removed when the
// transfer option in the kubernetes plugin is removed.
func (k *Kubernetes) TransferDeprecated(ctx context.Context, state request.Request) (int, error) {
	if !k.transferAllowed(state) {
		return dns.RcodeRefused, nil
	}

	// Get all services.
	rrss := make(chan []dns.RR)
	go k.transferAndClose(rrss, state.Zone)

	records := []dns.RR{}
	for rrs := range rrss {
		for _, rr := range rrs {
			records = append(records, rr)
		}
	}

	if len(records) == 0 {
		return dns.RcodeServerFailure, nil
	}

	ch := make(chan *dns.Envelope)
	tr := new(dns.Transfer)

	soa, err := plugin.SOA(ctx, k, state.Zone, state, plugin.Options{})
	if err != nil {
		return dns.RcodeServerFailure, nil
	}

	records = append(soa, records...)
	records = append(records, soa...)
	go func(ch chan *dns.Envelope) {
		j, l := 0, 0
		log.Infof("Outgoing transfer of %d records of zone %s to %s started", len(records), state.Zone, state.IP())
		for i, r := range records {
			l += dns.Len(r)
			if l > 2000 {
				ch <- &dns.Envelope{RR: records[j:i]}
				l = 0
				j = i
			}
		}
		if j < len(records) {
			ch <- &dns.Envelope{RR: records[j:]}
		}
		close(ch)
	}(ch)

	tr.Out(state.W, state.Req, ch)
	// Defer closing to the client
	state.W.Hijack()
	return dns.RcodeSuccess, nil
}

// transferAllowed checks if incoming request for transferring the zone is allowed according to the ACLs.
// This function is deprecated, replaced by the transfer plugin. It should be removed when the
// transfer option in the kubernetes plugin is removed.
func (k *Kubernetes) transferAllowed(state request.Request) bool {
	for _, t := range k.TransferTo {
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

// transferAndClose performs sends a transfer and closes the channel.
// This function is deprecated, replaced by the transfer plugin. It should be removed when the
// transfer option in the kubernetes plugin is removed.
func (k *Kubernetes) transferAndClose(c chan []dns.RR, zone string) {
	defer close(c)
	k.transfer(c, zone)
}

