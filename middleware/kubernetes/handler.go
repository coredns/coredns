package kubernetes

import (
	"errors"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// ServeDNS implements the middleware.Handler interface.
func (k Kubernetes) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(k.Name(), errors.New("can only deal with ClassINET"))
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	// Check that query matches one of the zones served by this middleware,
	// otherwise delegate to the next in the pipeline.
	zone := middleware.Zones(k.Zones).Matches(state.Name())
	if zone == "" {
		if state.Type() != "PTR" {
			return middleware.NextOrFailure(k.Name(), k.Next, ctx, w, r)
		}
		// If this is a PTR request, and the request is in a defined
		// pod/service cidr range, process the request in this middleware,
		// otherwise pass to next middleware.
		if !k.isRequestInReverseRange(state.Name()) {
			return middleware.NextOrFailure(k.Name(), k.Next, ctx, w, r)
		}
		// Set the zone to this specific request.
		zone = state.Name()
	}

	// TODO(miek): place contents of route-request back here.
	records, extra, _, err := k.routeRequest(zone, state)

	if k.IsNameError(err) {
		if k.Fallthrough {
			return middleware.NextOrFailure(k.Name(), k.Next, ctx, w, r)
		}
		// Make err nil when returning here, so we don't log spam for NXDOMAIN.
		return middleware.BackendError(&k, zone, dns.RcodeNameError, state, nil /*debug*/, nil /* err */, middleware.Options{})
	}
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	if len(records) == 0 {
		return middleware.BackendError(&k, zone, dns.RcodeSuccess, state, nil /*debug*/, nil, middleware.Options{})
	}

	m.Answer = append(m.Answer, records...)
	m.Extra = append(m.Extra, extra...)

	m = dnsutil.Dedup(m)
	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (k *Kubernetes) routeRequest(zone string, state request.Request) (records []dns.RR, extra []dns.RR, debug []dns.RR, err error) {
	switch state.Type() {
	case "A":
		records, _, err = middleware.A(k, zone, state, nil, middleware.Options{})
	case "AAAA":
		records, _, err = middleware.AAAA(k, zone, state, nil, middleware.Options{})
	case "TXT":
		records, _, err = middleware.TXT(k, zone, state, middleware.Options{})
	case "CNAME":
		records, _, err = middleware.CNAME(k, zone, state, middleware.Options{})
	case "PTR":
		records, _, err = middleware.PTR(k, zone, state, middleware.Options{})
	case "MX":
		records, extra, _, err = middleware.MX(k, zone, state, middleware.Options{})
	case "SRV":
		records, extra, _, err = middleware.SRV(k, zone, state, middleware.Options{})
	case "SOA":
		records, _, err = middleware.SOA(k, zone, state, middleware.Options{})
	case "NS":
		if state.Name() == zone {
			records, extra, _, err = middleware.NS(k, zone, state, middleware.Options{})
			break
		}
		fallthrough
	default:
		// Do a fake A lookup, so we can distinguish between NODATA and NXDOMAIN
		_, _, err = middleware.A(k, zone, state, nil, middleware.Options{})
	}
	return records, extra, nil, err
}

// Name implements the Handler interface.
func (k Kubernetes) Name() string { return "kubernetes" }
