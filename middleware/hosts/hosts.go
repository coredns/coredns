package hosts

import (
	"errors"
	"net"

	"golang.org/x/net/context"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/metrics"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type (
	Hosts struct {
		Next middleware.Handler
		*Hostsfile

		metrics *metrics.Metrics
	}
)

func (h Hosts) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(h.Name(), errors.New("can only deal with ClassINET"))
	}
	qname := state.Name()

	// Now the real zone.
	zone := middleware.Zones(h.Names()).Matches(qname)

	answers := []dns.RR{}

	switch state.QType() {
	case dns.TypePTR:
		names := h.LookupStaticAddr(dnsutil.ExtractAddressFromReverse(zone))
		if len(names) == 0 {
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		for _, n := range names {
			r := new(dns.PTR)
			r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypePTR,
				Class: dns.ClassINET, Ttl: 3600}
			r.Ptr = n
			answers = append(answers, r)
		}
	case dns.TypeA:
		strings := h.LookupStaticHost(zone)
		if len(strings) == 0 {
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		for _, str := range strings {
			ip := net.ParseIP(str)
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			r := new(dns.A)
			r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA,
				Class: dns.ClassINET, Ttl: 3600}
			r.A = ip
			answers = append(answers, r)
		}
	case dns.TypeAAAA:
		strings := h.LookupStaticHost(zone)
		if len(strings) == 0 {
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		for _, str := range strings {
			ip := net.ParseIP(str)
			if ip == nil {
				continue
			}
			ipv4 := ip.To4()
			if ipv4 != nil {
				continue
			}
			r := new(dns.AAAA)
			r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeAAAA,
				Class: dns.ClassINET, Ttl: 3600}
			r.AAAA = ip
			answers = append(answers, r)
		}
	default:
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	m.Answer = answers

	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (h Hosts) Name() string { return "hosts" }
