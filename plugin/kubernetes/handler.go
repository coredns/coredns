package kubernetes

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// ServeDNS implements the plugin.Handler interface.
func (k Kubernetes) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.QName()
	zone := plugin.Zones(k.Zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(k.Name(), k.Next, ctx, w, r)
	}
	zone = qname[len(qname)-len(zone):] // maintain case of original query
	state.Zone = zone

	var (
		records   []dns.RR
		extra     []dns.RR
		truncated bool
		err       error
	)

	switch state.QType() {
	case dns.TypeA:
		records, truncated, err = plugin.A(ctx, &k, zone, state, nil, plugin.Options{})
	case dns.TypeAAAA:
		records, truncated, err = plugin.AAAA(ctx, &k, zone, state, nil, plugin.Options{})
	case dns.TypeTXT:
		records, truncated, err = plugin.TXT(ctx, &k, zone, state, nil, plugin.Options{})
	case dns.TypeCNAME:
		records, err = plugin.CNAME(ctx, &k, zone, state, plugin.Options{})
	case dns.TypePTR:
		records, err = plugin.PTR(ctx, &k, zone, state, plugin.Options{})
	case dns.TypeMX:
		records, extra, err = plugin.MX(ctx, &k, zone, state, plugin.Options{})
	case dns.TypeSRV:
		records, extra, err = plugin.SRV(ctx, &k, zone, state, plugin.Options{})
	case dns.TypeSOA:
		if qname == zone {
			records, err = plugin.SOA(ctx, &k, zone, state, plugin.Options{})
		}
	case dns.TypeAXFR, dns.TypeIXFR:
		return dns.RcodeRefused, nil
	case dns.TypeNS:
		if state.Name() == zone {
			records, extra, err = plugin.NS(ctx, &k, zone, state, plugin.Options{})
			break
		}
		fallthrough
	default:
		// Do a fake A lookup, so we can distinguish between NODATA and NXDOMAIN
		fake := state.NewWithQuestion(state.QName(), dns.TypeA)
		fake.Zone = state.Zone
		_, _, err = plugin.A(ctx, &k, zone, fake, nil, plugin.Options{})
	}

	if k.IsNameError(err) {
		if k.Fall.Through(state.Name()) {
			return plugin.NextOrFailure(k.Name(), k.Next, ctx, w, r)
		}
		if !k.APIConn.HasSynced() {
			// If we haven't synchronized with the kubernetes cluster, return server failure
			return plugin.BackendError(ctx, &k, zone, dns.RcodeServerFailure, state, nil /* err */, plugin.Options{})
		}
		return plugin.BackendError(ctx, &k, zone, dns.RcodeNameError, state, nil /* err */, plugin.Options{})
	}
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	if len(records) == 0 {
		return plugin.BackendError(ctx, &k, zone, dns.RcodeSuccess, state, nil, plugin.Options{})
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Truncated = truncated
	m.Authoritative = true
	m.Answer = append(m.Answer, records...)
	m.Extra = append(m.Extra, extra...)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (k Kubernetes) Name() string { return "kubernetes" }
