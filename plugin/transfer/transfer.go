package transfer

import (
	"context"
	"errors"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("transfer")

// Transfer is a plugin that handles zone transfers.
type Transfer struct {
	Transferers []Transferer // the list of plugins that implement Transferer
	xfrs []*xfr
	Next plugin.Handler // the next plugin in the chain
}

type xfr struct {
	Zones       []string
	to          []string
}

// Transferer may be implemented by plugins to enable zone transfers
type Transferer interface {
	// Transfer returns a channel to which it writes responses to the transfer request.
	// If the plugin is not authoritative for the zone, it should immediately return the
	// Transfer.ErrNotAuthoritative error.
	//
	// If serial is 0, handle as an AXFR request. Transfer should send all records
	// in the zone to the channel. The SOA should be written to the channel first, followed
	// by all other records, including all NS + glue records.
	//
	// If serial is not 0, handle as an IXFR request. If the serial is >
	// the current serial for the zone, send a single SOA record to the channel.
	// If the serial is >= the current serial for the zone, perform an AXFR fallback
	// by proceeding as if an AXFR was requested (as above).
	Transfer(zone string, serial uint32) (<-chan []dns.RR, error)
}

var (
	// ErrNotAuthoritative is returned by Transfer() when the plugin is not authoritative for the zone
	ErrNotAuthoritative = errors.New("not authoritative for zone")
)

// ServeDNS implements the plugin.Handler interface.
func (t Transfer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QType() != dns.TypeAXFR  && state.QType() != dns.TypeIXFR {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

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

	// Get serial from request if this is an IXFR
	var serial uint32
	if state.QType() == dns.TypeIXFR {
		soa, ok := r.Ns[0].(*dns.SOA)
		if !ok {
			return dns.RcodeServerFailure, nil
		}
		serial = soa.Serial
	}

	// Try each Transferer plugin
	for _, p := range t.Transferers {
		ch, err := p.Transfer(zone, serial)
		if err == ErrNotAuthoritative {
			// plugin was not authoritative for the zone, try next plugin
			continue
		}
		if err != nil {
			return dns.RcodeServerFailure, nil
		}
		x.send(state, ch)
		for range ch {
			// drain rest of channel if sendAxfr was aborted
		}
		return dns.RcodeSuccess, nil
	}

	return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)

}

// send sends the response to the client.  It aborts if the first record received on the
// channel is not an SOA.  send automatically adds the closing SOA to the end of an AXFR response.
func (x xfr) send(state request.Request, fromPlugin <-chan []dns.RR) {
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

		if c > 1 {
			// add closing SOA
			c++
			records = append(records, soa)
		}

		if len(records) > 0 {
			// write remaining records
			toRemote <- &dns.Envelope{RR: records}
		}

		close(toRemote)
		log.Infof("Outgoing transfer of %d records of zone %s to %s with %d SOA serial", c, state.QName(), state.IP(), serial)
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
