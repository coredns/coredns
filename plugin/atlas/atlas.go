// package atlas is a CoreDNS plugin that prints "atlas" to stdout on every packet received.
//
// It serves as an atlas CoreDNS plugin with numerous code comments.
package atlas

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Atlas is an database plugin.
type Atlas struct {
	Next plugin.Handler

	cfg            *Config
	zones          []string
	lastZoneUpdate time.Time
}

// ServeDNS implements the plugin.Handler interface. This method gets called when atlas is used
// in a Server.
//
// A records cause no additional section processing.
//
// MX records cause type A additional section processing for the host specified by EXCHANGE.
//
// CNAME RRs cause no additional section processing, but name servers may
// choose to restart the query at the canonical name in certain cases.  See
// the description of name server logic in [RFC-1034](https://datatracker.ietf.org/doc/html/rfc1034) for details.
//
// NS records cause both the usual additional section processing to locate
// a type A record, and, when used in a referral, a special search of the
// zone in which they reside for glue information.
//
// PTR records cause no additional section processing.
//
// SOA records cause no additional section processing.
func (a *Atlas) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	responseMsg := new(dns.Msg)
	responseMsg.SetReply(r)
	responseMsg.Authoritative = true
	responseMsg.RecursionAvailable = false
	responseMsg.Compress = true

	state := request.Request{W: w, Req: r}

	reqName := state.Name()
	reqType := state.QType()

	// Export metric with the server label set to the current server handling the request.
	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	client, err := a.getAtlasClient()
	if err != nil {
		return a.errorResponse(state, dns.RcodeServerFailure, err)
	}
	defer client.Close()

	if a.mustReloadZones() {
		err := a.loadZones(ctx, client)
		if err != nil {
			return a.errorResponse(state, dns.RcodeServerFailure, err)
		}
	}

	reqZone := plugin.Zones(a.zones).Matches(reqName)
	if reqZone == "" {
		return plugin.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	// TODO(jproxx): implement axfr requests, because we are needing them
	if reqType == dns.TypeAXFR {
		return a.errorResponse(state, dns.RcodeNotImplemented, nil)
	}

	// handle soa record
	var soaRec []dns.RR
	if reqType == dns.TypeSOA {
		soaRec, err = a.getSOARecord(ctx, client, reqZone)
		if err != nil {
			return a.errorResponse(state, dns.RcodeServerFailure, err)
		}
		responseMsg.Answer = append(responseMsg.Answer, soaRec...)

		// TODO(jproxx): make it dry
		state.SizeAndDo(responseMsg)
		responseMsg = state.Scrub(responseMsg)
		if err = w.WriteMsg(responseMsg); err != nil {
			return dns.RcodeServerFailure, err
		}

		return dns.RcodeSuccess, nil
	}

	rrs, err := a.getRRecords(ctx, client, reqName, reqType)
	if err != nil {
		return a.errorResponse(state, dns.RcodeServerFailure, err)
	}

	if len(rrs) == 0 {
		// to get bind compliance we are returning the SOA rr
		soaRec, err = a.getSOARecord(ctx, client, reqZone)
		if err != nil {
			return a.errorResponse(state, dns.RcodeServerFailure, err)
		}
		responseMsg.Answer = append(responseMsg.Answer, soaRec...)

		// TODO(jproxx): make it dry
		state.SizeAndDo(responseMsg)
		responseMsg = state.Scrub(responseMsg)
		if err = w.WriteMsg(responseMsg); err != nil {
			return dns.RcodeServerFailure, err
		}

		return dns.RcodeSuccess, nil
	}

	responseMsg.Answer = append(responseMsg.Answer, rrs...)

	// build authoritative section
	// TODO(jproxx): check, when we have to provide a authoritative section + additional section!
	if nsRRs, err := a.getNameServers(ctx, client, reqZone); err != nil {
		return a.errorResponse(state, dns.RcodeServerFailure, err)
	} else {
		responseMsg.Ns = nsRRs
	}

	// TODO(jproxx): make it dry
	state.SizeAndDo(responseMsg)
	responseMsg = state.Scrub(responseMsg)
	if err = w.WriteMsg(responseMsg); err != nil {
		return dns.RcodeServerFailure, err
	}

	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (a *Atlas) Name() string { return plgName }

func (a *Atlas) mustReloadZones() bool {
	return time.Since(a.lastZoneUpdate) > a.cfg.zoneUpdateTime
}

func (handler *Atlas) errorResponse(state request.Request, rCode int, err error) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rCode)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	state.SizeAndDo(m)
	if writeErr := state.W.WriteMsg(m); writeErr != nil {
		return dns.RcodeServerFailure, err
	}
	// Return success as the rCode to signal we have written to the client.
	return dns.RcodeSuccess, err
}

// ResponsePrinter wrap a dns.ResponseWriter and will write atlas to standard output when WriteMsg is called.
type ResponsePrinter struct {
	dns.ResponseWriter
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter) *ResponsePrinter {
	return &ResponsePrinter{ResponseWriter: w}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "atlas" to standard output.
func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	return r.ResponseWriter.WriteMsg(res)
}
