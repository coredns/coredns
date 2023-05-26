package local

import (
	"context"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("local")

// Local is a plugin that returns standard replies for local queries.
type Local struct {
	Next plugin.Handler
}

var zones = []string{"localhost.", "0.in-addr.arpa.", "127.in-addr.arpa.", "255.in-addr.arpa."}

func soaFromOrigin(origin string) []dns.RR {
	hdr := dns.RR_Header{Name: origin, Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeSOA}
	return []dns.RR{&dns.SOA{Hdr: hdr, Ns: "localhost.", Mbox: "root.localhost.", Serial: 1, Refresh: 0, Retry: 0, Expire: 0, Minttl: ttl}}
}

func nsFromOrigin(origin string) []dns.RR {
	hdr := dns.RR_Header{Name: origin, Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeNS}
	return []dns.RR{&dns.NS{Hdr: hdr, Ns: "localhost."}}
}

// ServeDNS implements the plugin.Handler interface.
func (l Local) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.QName()

	lc := len("localhost.")
	if len(state.Name()) > lc && strings.HasPrefix(state.Name(), "localhost.") {
		// we have multiple labels, but the first one is localhost, intercept this and return 127.0.0.1 or ::1
		log.Debugf("Intercepting localhost query for %q %s, from %s", state.Name(), state.Type(), state.IP())
		LocalhostCount.Inc()
		reply := doLocalhost(state)
		w.WriteMsg(reply)
		return 0, nil
	}

	zone := plugin.Zones(zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(l.Name(), l.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	zone = qname[len(qname)-len(zone):]

	switch q := state.Name(); q {
	case "localhost.", "0.in-addr.arpa.", "127.in-addr.arpa.", "255.in-addr.arpa.":
		switch state.QType() {
		case dns.TypeA:
			if q != "localhost." {
				// nodata
				m.Ns = soaFromOrigin(qname)
				break
			}

			hdr := dns.RR_Header{Name: qname, Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeA}
			m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: net.ParseIP("127.0.0.1").To4()}}
		case dns.TypeAAAA:
			if q != "localhost." {
				// nodata
				m.Ns = soaFromOrigin(qname)
				break
			}

			hdr := dns.RR_Header{Name: qname, Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeAAAA}
			m.Answer = []dns.RR{&dns.AAAA{Hdr: hdr, AAAA: net.ParseIP("::1")}}
		case dns.TypeSOA:
			m.Answer = soaFromOrigin(qname)
		case dns.TypeNS:
			m.Answer = nsFromOrigin(qname)
		default:
			// nodata
			m.Ns = soaFromOrigin(qname)
		}
	case "1.0.0.127.in-addr.arpa.":
		switch state.QType() {
		case dns.TypePTR:
			hdr := dns.RR_Header{Name: qname, Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypePTR}
			m.Answer = []dns.RR{&dns.PTR{Hdr: hdr, Ptr: "localhost."}}
		default:
			// nodata
			m.Ns = soaFromOrigin(zone)
		}
	}

	if len(m.Answer) == 0 && len(m.Ns) == 0 {
		m.Ns = soaFromOrigin(zone)
		m.Rcode = dns.RcodeNameError
	}

	w.WriteMsg(m)
	return 0, nil
}

// Name implements the plugin.Handler interface.
func (l Local) Name() string { return "local" }

func doLocalhost(state request.Request) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(state.Req)
	switch state.QType() {
	case dns.TypeA:
		hdr := dns.RR_Header{Name: state.QName(), Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeA}
		m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: net.ParseIP("127.0.0.1").To4()}}
	case dns.TypeAAAA:
		hdr := dns.RR_Header{Name: state.QName(), Ttl: ttl, Class: dns.ClassINET, Rrtype: dns.TypeAAAA}
		m.Answer = []dns.RR{&dns.AAAA{Hdr: hdr, AAAA: net.ParseIP("::1")}}
	default:
		// nodata
		m.Ns = soaFromOrigin(state.QName())
	}
	return m
}

const ttl = 604800
