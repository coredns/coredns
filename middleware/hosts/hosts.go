package hosts

import (
	"errors"
	"net"

	"golang.org/x/net/context"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Hosts is the middleware handler
type Hosts struct {
	Next middleware.Handler
	*Hostsfile

	Origins     []string
	Fallthrough bool
}

// ServeDNS implements the middleware.Handle interface.
func (h Hosts) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(h.Name(), errors.New("can only deal with ClassINET"))
	}
	qname := state.Name()

	answers := []dns.RR{}

	// Precheck with the origins, i.e. are we allowed to looks here.
	if h.Origins != nil {
		zone := middleware.Zones(h.Origins).Matches(qname)
		if zone == "" {
			// PTR zones don't need to be specified in Origins
			if state.Type() != "PTR" {
				// If this doesn't match we need to fall through regardless of h.Fallthrough
				return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
			}
		}
	}

	zone := middleware.Zones(h.Names()).Matches(qname)
	if zone == "" {
		if state.Type() != "PTR" {
			return h.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}

		// Request is a PTR
		zone = state.Name()
		names := h.LookupStaticAddr(dnsutil.ExtractAddressFromReverse(zone))
		if len(names) == 0 {
			// If this doesn't match we need to fall through regardless of h.Fallthrough
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		answers = h.ptr(zone, names)
	}

	if zone != "" {
		ips := h.LookupStaticHost(zone)

		switch state.QType() {
		case dns.TypeA:
			answers = a(zone, ips)
		case dns.TypeAAAA:
			answers = aaaa(zone, ips)
		}
	}

	if len(answers) == 0 {
		return h.NextOrFailure(h.Name(), h.Next, ctx, w, r)
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

// Name implements the middleware.Handle interface.
func (h Hosts) Name() string { return "hosts" }

// NextOrFailure calls middleware.NextOrFailure if h.Fallthrough is set.
// If it is not set, we just return a dns.RcodeRefused.
func (h Hosts) NextOrFailure(name string, next middleware.Handler, ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if h.Fallthrough {
		return middleware.NextOrFailure(name, next, ctx, w, r)
	}
	return dns.RcodeRefused, nil
}

// ipv6Filter parses a slice of strings into a slice of net.IP and filters out the ipv6 ips.
func ipv4Filter(strings []string) []net.IP {
	ips := []net.IP{}
	for _, str := range strings {
		ip := net.ParseIP(str)
		if ip == nil {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

// ipv6Filter parses a slice of strings into a slice of net.IP and filters out the ipv4 ips.
func ipv6Filter(strings []string) []net.IP {
	ips := []net.IP{}
	for _, str := range strings {
		ip := net.ParseIP(str)
		if ip == nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 != nil {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

// a takes a slice of ip strings, parses them, filters out the non-ipv4 ips, and returns a slice of A RRs.
func a(zone string, ips []string) []dns.RR {
	answers := []dns.RR{}
	for _, ip := range ipv4Filter(ips) {
		r := new(dns.A)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA,
			Class: dns.ClassINET, Ttl: 3600}
		r.A = ip
		answers = append(answers, r)
	}
	return answers
}

// aaaa takes a slice of ip strings, parses them, filters out the non-ipv6 ips, and returns a slice of AAAA RRs.
func aaaa(zone string, ips []string) []dns.RR {
	answers := []dns.RR{}
	for _, ip := range ipv6Filter(ips) {
		r := new(dns.AAAA)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeAAAA,
			Class: dns.ClassINET, Ttl: 3600}
		r.AAAA = ip
		answers = append(answers, r)
	}
	return answers
}

// ptr takes a slice of host names and filters out the ones that aren't in Origins, if specified, and returns a slice of PTR RRs.
func (h *Hosts) ptr(zone string, names []string) []dns.RR {
	answers := []dns.RR{}
	for _, n := range names {
		if h.Origins != nil {
			// Filter out zones that we are not authoritive for
			zone := middleware.Zones(h.Origins).Matches(n)
			if zone == "" {
				continue
			}
		}
		r := new(dns.PTR)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypePTR,
			Class: dns.ClassINET, Ttl: 3600}
		r.Ptr = dns.Fqdn(n)
		answers = append(answers, r)
	}
	return answers
}
