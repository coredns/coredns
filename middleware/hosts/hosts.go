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

type (
	Hosts struct {
		Next middleware.Handler
		*Hostsfile
	}
)

func (h Hosts) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(h.Name(), errors.New("can only deal with ClassINET"))
	}
	qname := state.Name()

	answers := []dns.RR{}

	zone := middleware.Zones(h.Names()).Matches(qname)
	if zone == "" {
		if state.Type() != "PTR" {
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}

		// Request is a PTR
		zone = state.Name()
		names := h.LookupStaticAddr(dnsutil.ExtractAddressFromReverse(zone))
		if len(names) == 0 {
			return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		answers = ptr(zone, names)
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
		return middleware.NextOrFailure(h.Name(), h.Next, ctx, w, r)
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

func ptr(zone string, names []string) []dns.RR {
	answers := []dns.RR{}
	for _, n := range names {
		r := new(dns.PTR)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypePTR,
			Class: dns.ClassINET, Ttl: 3600}
		r.Ptr = dns.Fqdn(n)
		answers = append(answers, r)
	}
	return answers
}
