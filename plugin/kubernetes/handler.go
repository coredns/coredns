package kubernetes

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

type Handler struct {
	Zones      []string
	Next       plugin.Handler
	Kubernetes []*Kubernetes
}

// ServeDNS implements the plugin.Handler interface.
func (h Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true

	var (
		records []dns.RR
		extra   []dns.RR
		err     error
	)

	for _, k := range h.Kubernetes {
		zone := plugin.Zones(k.Zones).Matches(state.Name())
		if zone == "" {
			continue
		}

		state.Zone = zone

		switch state.QType() {
		case dns.TypeA:
			records, err = plugin.A(k, zone, state, nil, plugin.Options{})
		case dns.TypeAAAA:
			records, err = plugin.AAAA(k, zone, state, nil, plugin.Options{})
		case dns.TypeTXT:
			records, err = plugin.TXT(k, zone, state, plugin.Options{})
		case dns.TypeCNAME:
			records, err = plugin.CNAME(k, zone, state, plugin.Options{})
		case dns.TypePTR:
			records, err = plugin.PTR(k, zone, state, plugin.Options{})
		case dns.TypeMX:
			records, extra, err = plugin.MX(k, zone, state, plugin.Options{})
		case dns.TypeSRV:
			records, extra, err = plugin.SRV(k, zone, state, plugin.Options{})
		case dns.TypeSOA:
			records, err = plugin.SOA(k, zone, state, plugin.Options{})
		case dns.TypeNS:
			if state.Name() == zone {
				records, extra, err = plugin.NS(k, zone, state, plugin.Options{})
				break
			}
			fallthrough
		default:
			// Do a fake A lookup, so we can distinguish between NODATA and NXDOMAIN
			_, err = plugin.A(k, zone, state, nil, plugin.Options{})
		}

		if k.IsNameError(err) {
			if k.Fall.Through(state.Name()) {
				continue
			}
			return plugin.BackendError(k, zone, dns.RcodeNameError, state, nil /* err */, plugin.Options{})
		}
		if err != nil {
			return dns.RcodeServerFailure, err
		}

		if len(records) == 0 {
			return plugin.BackendError(k, zone, dns.RcodeSuccess, state, nil, plugin.Options{})
		}

		m.Answer = append(m.Answer, records...)
		m.Extra = append(m.Extra, extra...)

		m = dnsutil.Dedup(m)
		state.SizeAndDo(m)
		m, _ = state.Scrub(m)
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name implements the Handler interface.
func (h Handler) Name() string { return "kubernetes" }
