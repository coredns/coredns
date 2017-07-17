package etcd

import (
	"errors"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/backend/msg"
	"github.com/coredns/coredns/middleware/pkg/debug"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// ServeDNS implements the middleware.Handler interface.
func (b *Backend) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	opt := middleware.Options{}
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(b.ServiceName, errors.New("can only deal with ClassINET"))
	}
	name := state.Name()
	if b.Debugging {
		if bug := debug.IsDebug(name); bug != "" {
			opt.Debug = r.Question[0].Name
			state.Clear()
			state.Req.Question[0].Name = bug
		}
	}

	// We need to check stubzones first, because we may get a request for a zone we
	// are not auth. for *but* do have a stubzone forward for. If we do the stubzone
	// handler will handle the request.
	if b.Stubmap != nil && len(*b.Stubmap) > 0 {
		for zone := range *b.Stubmap {
			if middleware.Name(zone).Matches(name) {
				stub := Stub{Backend: b, Zone: zone}
				return stub.ServeDNS(ctx, w, r)
			}
		}
	}

	zone := middleware.Zones(b.Zones).Matches(state.Name())
	if zone == "" {
		if opt.Debug != "" {
			r.Question[0].Name = opt.Debug
		}
		return middleware.NextOrFailure(b.ServiceName, b.Next, ctx, w, r)
	}

	var (
		records, extra []dns.RR
		debug          []msg.Service
		err            error
	)
	switch state.Type() {
	case "A":
		records, debug, err = middleware.A(b.ServiceBackend, zone, state, nil, opt)
	case "AAAA":
		records, debug, err = middleware.AAAA(b.ServiceBackend, zone, state, nil, opt)
	case "TXT":
		records, debug, err = middleware.TXT(b.ServiceBackend, zone, state, opt)
	case "CNAME":
		records, debug, err = middleware.CNAME(b.ServiceBackend, zone, state, opt)
	case "PTR":
		records, debug, err = middleware.PTR(b.ServiceBackend, zone, state, opt)
	case "MX":
		records, extra, debug, err = middleware.MX(b.ServiceBackend, zone, state, opt)
	case "SRV":
		records, extra, debug, err = middleware.SRV(b.ServiceBackend, zone, state, opt)
	case "SOA":
		records, debug, err = middleware.SOA(b.ServiceBackend, zone, state, opt)
	case "NS":
		if state.Name() == zone {
			records, extra, debug, err = middleware.NS(b.ServiceBackend, zone, state, opt)
			break
		}
		fallthrough
	default:
		// Do a fake A lookup, so we can distinguish between NODATA and NXDOMAIN
		_, debug, err = middleware.A(b.ServiceBackend, zone, state, nil, opt)
	}

	if opt.Debug != "" {
		// Substitute this name with the original when we return the request.
		state.Clear()
		state.Req.Question[0].Name = opt.Debug
	}

	if b.ServiceBackend.IsNameError(err) {
		// Make err nil when returning here, so we don't log spam for NXDOMAIN.
		return middleware.BackendError(b.ServiceBackend, zone, dns.RcodeNameError, state, debug, nil /* err */, opt)
	}
	if err != nil {
		return middleware.BackendError(b.ServiceBackend, zone, dns.RcodeServerFailure, state, debug, err, opt)
	}

	if len(records) == 0 {
		return middleware.BackendError(b.ServiceBackend, zone, dns.RcodeSuccess, state, debug, err, opt)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	m.Answer = append(m.Answer, records...)
	m.Extra = append(m.Extra, extra...)
	if opt.Debug != "" {
		m.Extra = append(m.Extra, middleware.ServicesToTxt(debug)...)
	}

	m = dnsutil.Dedup(m)
	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (b *Backend) Name() string { return b.ServiceName }
