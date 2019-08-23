package transfer

import (
	"context"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("transfer")

// Transfer is a plugin that handles zone transfers.
type Transfer struct {
	xfrs []*xfr
	Next plugin.Handler
}

type xfr struct {
	Zones       []string
	to          []string
	Transferers map[string]Transferer // a map of zone->plugins which implement Transferer
}

// Transferer may be implemented by plugins to enable zone transfers
type Transferer interface {
	// AllRecords returns a channel to which it writes all records in the zone.
	// All NS + glue records should be included
	// The SOA should not be written to the channel (will be ignored).
	AllRecords(zone string) <-chan []dns.RR

	// SOA returns the SOA  for the zone
	SOA(zone string) *dns.SOA

	// Authoritative must return a list of Zones that the plugin is authoritative for.
	Authoritative() []string
}

// ServeDNS implements the plugin.Handler interface.
func (t Transfer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	// Find the first transfer instance matching the zone.
	var x *xfr
	var zone string
	for _, xfr := range t.xfrs {
		zone = plugin.Zones(xfr.Zones).Matches(state.Name())
		if zone == "" {
			continue
		}
		x = xfr
	}

	if x == nil {
		// Requested zone did not match any transfer instance zones.
		// Pass request down chain in case later plugins are capable of handling transfer requests themselves.
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	if !x.allowed(state) {
		return dns.RcodeRefused, nil
	}

	// Get the plugin that is authoritative for the zone
	p, exists := x.Transferers[zone]
	if !exists {
		// Pass request down chain in case later plugins are capable of handling transfer requests themselves.
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	switch state.QType() {
	case dns.TypeIXFR:
		rSoa, ok := r.Ns[0].(*dns.SOA)
		if !ok {
			return dns.RcodeServerFailure, nil
		}
		cSoa := p.SOA(zone)

		if cSoa != nil && rSoa.Serial >= cSoa.Serial {
			// Per RFC 1995, section 2, para 4: If an IXFR query with the same or newer
			// version number than that of the server is received, we can send the simple
			// zone-current response (single SOA)
			x.sendIxfrCurrent(state, cSoa)
		}
		// Current serial didnt match, we dont implement IXFR zone change responses,
		// so fallback to AXFR style response per RFC 1995, section 4, para 1
		fallthrough
	case dns.TypeAXFR:
		ch := p.AllRecords(zone)
		x.sendAxfr(state, ch)
		for range ch {
			// drain rest of channel if sendAxfr was aborted
		}
		return dns.RcodeSuccess, nil
	}

	return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
}

// sendIxfrCurrent sends a simple "zone-current" response signaling to the client that their zone
// is the latest. Per RFC 1995, section 2, para 4 this is a single SOA record containing the
// server's current SOA
func (x xfr) sendIxfrCurrent(state request.Request, soa *dns.SOA) {
	toRemote := make(chan *dns.Envelope)
	go func() {
		toRemote <- &dns.Envelope{RR: []dns.RR{soa}}
		close(toRemote)
	}()
	tr := new(dns.Transfer)
	tr.Out(state.W, state.Req, toRemote)
	state.W.Hijack()
}

// sendAxfr sends the full zone to the client.  It aborts if the first record received on the
// channel is not an SOA.  sendAxfr automatically adds the closing SOA to the end of the response.
func (x xfr) sendAxfr(state request.Request, fromPlugin <-chan []dns.RR) {
	toRemote := make(chan *dns.Envelope)

	go func() {
		var records []dns.RR
		var serial uint32
		var soa *dns.SOA
		l, c := 0, 0
		for rs := range fromPlugin {
			for _, r := range rs {
				if c == 0 {
					var ok bool
					soa, ok = r.(*dns.SOA)
					if !ok {
						log.Errorf("First record in transfer from zone %s was not an SOA", state.QName())
						return
					}
					serial = soa.Serial
				}
				records = append(records, r)
				c++
				l += dns.Len(r)
				if l > 500 {
					toRemote <- &dns.Envelope{RR: records}
					l = 0
					records = nil
				}
			}
		}

		// write remaining records and closing SOA
		c++
		toRemote <- &dns.Envelope{RR: append(records, soa)}

		close(toRemote)
		log.Infof("Outgoing transfer of %d records of zone %s to %s started with %d SOA serial", c, state.QName(), state.IP(), serial)
	}()

	tr := new(dns.Transfer)
	tr.Out(state.W, state.Req, toRemote)
	state.W.Hijack()
}

func (x xfr) allowed(state request.Request) bool {
	for _, h := range x.to {
		if h == "*" {
			return true
		}
		// If remote IP matches we accept.
		remote := state.IP()
		if h == remote {
			return true
		}
	}
	return false
}

// Name implements the Handler interface.
func (Transfer) Name() string { return "transfer" }
