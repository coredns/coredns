package kubernetes

import (
	"fmt"

	"github.com/miekg/coredns/middleware"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func (k Kubernetes) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := middleware.State{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, fmt.Errorf("can only deal with ClassINET")
	}

	// We need to check stubzones first, because we may get a request for a zone we
	// are not auth. for *but* do have a stubzone forward for. If we do the stubzone
	// handler will handle the request.
    // TODO: Determine if stubzone support is needed
    /*
	name := state.Name()
	if k.Stubmap != nil && len(*k.Stubmap) > 0 {
		for zone, _ := range *k.Stubmap {
			if middleware.Name(zone).Matches(name) {
				stub := Stub{Kubernetes: k, Zone: zone}
				return stub.ServeDNS(ctx, w, r)
			}
		}
	}
    */


	zone := middleware.Zones(k.Zones).Matches(state.Name())
	if zone == "" {
		if k.Next == nil {
			return dns.RcodeServerFailure, nil
		}
		return k.Next.ServeDNS(ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true

	var (
		records, extra []dns.RR
		err            error
	)
	switch state.Type() {
	case "A":
		records, err = k.A(zone, state, nil)
	case "AAAA":
		records, err = k.AAAA(zone, state, nil)
	case "TXT":
		records, err = k.TXT(zone, state)
	case "CNAME":
		records, err = k.CNAME(zone, state)
	case "MX":
		records, extra, err = k.MX(zone, state)
	case "SRV":
		records, extra, err = k.SRV(zone, state)
	case "SOA":
		records = []dns.RR{k.SOA(zone, state)}
	case "NS":
		if state.Name() == zone {
			records, extra, err = k.NS(zone, state)
			break
		}
		fallthrough
	default:
		// Do a fake A lookup, so we can distinguish betwen NODATA and NXDOMAIN
		_, err = k.A(zone, state, nil)
	}
	if isKubernetesNameError(err) {
		return k.Err(zone, dns.RcodeNameError, state)
	}
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	if len(records) == 0 {
		return k.Err(zone, dns.RcodeSuccess, state)
	}

	m.Answer = append(m.Answer, records...)
	m.Extra = append(m.Extra, extra...)

	m = dedup(m)
	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// NoData write a nodata response to the client.
func (k Kubernetes) Err(zone string, rcode int, state middleware.State) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rcode)
	m.Ns = []dns.RR{k.SOA(zone, state)}
	state.SizeAndDo(m)
	state.W.WriteMsg(m)
	return rcode, nil
}

func dedup(m *dns.Msg) *dns.Msg {
	// TODO(miek): expensive!
	m.Answer = dns.Dedup(m.Answer, nil)
	m.Ns = dns.Dedup(m.Ns, nil)
	m.Extra = dns.Dedup(m.Extra, nil)
	return m
}
