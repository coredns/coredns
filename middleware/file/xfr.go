package file

import (
	"fmt"
	"log"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Xfr serves up an AXFR.
type Xfr struct {
	*Zone
}

// ServeDNS implements the middleware.Handler interface.
func (x Xfr) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if transfer.Allowed(state, x.TransferTo) {
		return dns.RcodeServerFailure, nil
	}
	if state.QType() != dns.TypeAXFR && state.QType() != dns.TypeIXFR {
		return 0, middleware.Error(x.Name(), fmt.Errorf("xfr called with non transfer type: %d", state.QType()))
	}

	records := x.All()
	if len(records) == 0 {
		return dns.RcodeServerFailure, nil
	}

	log.Printf("[INFO] Outgoing transfer of %d records of zone %s to %s started", len(records), x.origin, state.IP())
	records = append(records, records[0]) // add closing SOA to the end

	transfer.Out(state, records)

	return dns.RcodeSuccess, nil
}

// Name implements the middleware.Hander interface.
func (x Xfr) Name() string { return "xfr" }
