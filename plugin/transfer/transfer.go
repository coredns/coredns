package transfer

import (
	"context"
	"errors"
	"net"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("transfer")

// Transfer is a plugin that handles zone transfers.
type Transfer struct {
	Transferers []Transferer // List of plugins that implement Transferer
	xfrs        []*xfr
	Next        plugin.Handler
}

type xfr struct {
	Zones []string
	to    []string
}

// Transferer may be implemented by plugins to enable zone transfers
type Transferer interface {
	// Transfer returns a channel to which it writes responses to the transfer request.
	// If the plugin is not authoritative for the zone, it should immediately return the
	// transfer.ErrNotAuthoritative error. This is important otherwise the transfer plugin can
	// use plugin X while it should transfer the data from plugin Y.
	//
	// If serial is 0, handle as an AXFR request. Transfer should send all records
	// in the zone to the channel. The SOA should be written to the channel first, followed
	// by all other records, including all NS + glue records. The implemenation is also responsible
	// for sending the last SOA record (to signal end of the transfer). This plugin will just grab
	// these records and send them back to the requester, there is little validation done.
	//
	// If serial is not 0, it will be handled as an IXFR request. If the serial is equal to or greater (newer) than
	// the current serial for the zone, send a single SOA record to the channel and then close it.
	// If the serial is less (older) than the current serial for the zone, perform an AXFR fallback
	// by proceeding as if an AXFR was requested (as above).
	Transfer(zone string, serial uint32) (<-chan []dns.RR, error)
}

var (
	// ErrNotAuthoritative is returned by Transfer() when the plugin is not authoritative for the zone.
	ErrNotAuthoritative = errors.New("not authoritative for zone")
)

// ServeDNS implements the plugin.Handler interface.
func (t *Transfer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QType() != dns.TypeAXFR && state.QType() != dns.TypeIXFR {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	if state.Proto() != "tcp" {
		return dns.RcodeRefused, nil
	}

	x := longestMatch(t.xfrs, state.QName())
	if x == nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	if !x.allowed(state) {
		// write msg here, so logging will pick it up
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		w.WriteMsg(m)
		return 0, nil
	}

	// Get serial from request if this is an IXFR.
	var serial uint32
	if state.QType() == dns.TypeIXFR {
		if len(r.Ns) != 1 {
			return dns.RcodeServerFailure, nil
		}
		soa, ok := r.Ns[0].(*dns.SOA)
		if !ok {
			return dns.RcodeServerFailure, nil
		}
		serial = soa.Serial
	}

	// Get a receiving channel from the first Transferer plugin that returns one.
	var pchan <-chan []dns.RR
	var err error
	for _, p := range t.Transferers {
		pchan, err = p.Transfer(state.QName(), serial)
		if err == ErrNotAuthoritative {
			// plugin was not authoritative for the zone, try next plugin
			continue
		}
		if err != nil {
			return dns.RcodeServerFailure, err
		}
		break
	}

	if pchan == nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	// Send response to client
	ch := make(chan *dns.Envelope)
	tr := new(dns.Transfer)
	errCh := make(chan error)
	go func() {
		if err := tr.Out(w, r, ch); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	rrs := []dns.RR{}
	l := 0
	var soa *dns.SOA
	for records := range pchan {
		if x, ok := records[0].(*dns.SOA); ok && soa == nil {
			soa = x
		}
		rrs = append(rrs, records...)
		if len(rrs) > 500 {
			select {
			case ch <- &dns.Envelope{RR: rrs}:
			case err := <-errCh:
				return dns.RcodeServerFailure, err
			}
			l += len(rrs)
			rrs = []dns.RR{}
		}
	}

	// if we are here and we only hold 1 soa (len(rrs) == 1) and soa != nil, and IXFR fallback should
	// be performed. We haven't send anything on ch yet, so that can be closed (and waited for), and we only
	// need to return the SOA back to the client and return.
	if len(rrs) == 1 && soa != nil { // soa should never be nil...
		close(ch)
		err := <-errCh
		if err != nil {
			return dns.RcodeServerFailure, err
		}

		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{soa}
		w.WriteMsg(m)

		log.Infof("Outgoing noop, incremental transfer for up to date zone %q to %s for %d SOA serial", state.QName(), state.IP(), soa.Serial)
		return 0, nil
	}

	if len(rrs) > 0 {
		select {
		case ch <- &dns.Envelope{RR: rrs}:
		case err := <-errCh:
			return dns.RcodeServerFailure, err
		}
		l += len(rrs)

	}

	close(ch)     // Even though we close the channel here, we still have
	err = <-errCh // to wait before we can return and close the connection.
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	logserial := uint32(0)
	if soa != nil {
		logserial = soa.Serial
	}
	log.Infof("Outgoing transfer of %d records of zone %q to %s for %d SOA serial", l, state.QName(), state.IP(), logserial)
	return 0, nil
}

func (x xfr) allowed(state request.Request) bool {
	for _, h := range x.to {
		if h == "*" {
			return true
		}
		to, _, err := net.SplitHostPort(h)
		if err != nil {
			return false
		}
		// If remote IP matches we accept. TODO(): make this works with ranges
		if to == state.IP() {
			return true
		}
	}
	return false
}

// Find the first transfer instance for which the queried zone is the longest match. When nothing
// is found nil is returned.
func longestMatch(xfrs []*xfr, name string) *xfr {
	// TODO(xxx): optimize and make it a map (or maps)
	var x *xfr
	zone := "" // longest zone match wins
	for _, xfr := range xfrs {
		if z := plugin.Zones(xfr.Zones).Matches(name); z != "" {
			if z > zone {
				zone = z
				x = xfr
			}
		}
	}
	return x
}

// Name implements the Handler interface.
func (Transfer) Name() string { return "transfer" }
