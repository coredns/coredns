package whoareyou

import (
	"net"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/whoareyou/ipscope"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

type Whoareyou struct {
	Next   middleware.Handler
	scopes *ipscope.IPScopes
}

func (wh Whoareyou) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	a := new(dns.Msg)
	a.SetReply(r)
	a.Compress = true
	a.Authoritative = true

	var ip net.IP
	switch addr := state.W.RemoteAddr().(type) {
	case *net.UDPAddr:
		ip = addr.IP
	case *net.TCPAddr:
		ip = addr.IP
	}
	var port int
	switch addr := state.W.LocalAddr().(type) {
	case *net.UDPAddr:
		port = addr.Port
	case *net.TCPAddr:
		port = addr.Port
	}

	var rrs []dns.RR

	switch state.Family() {
	case 1:
		if state.QType() == dns.TypeAAAA {
			break
		}
		ipset4 := wh.scopes.Get(ip).To4()
		for _, ip4 := range ipset4 {
			rr := new(dns.A)
			rr.Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: state.QClass()}
			rr.A = ip4
			rrs = append(rrs, rr)
		}
	case 2:
		if state.QType() == dns.TypeA {
			break
		}
		ipset6 := wh.scopes.Get(ip).To6()
		for _, ip6 := range ipset6 {
			rr := new(dns.AAAA)
			rr.Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: state.QClass()}
			rr.AAAA = ip6
			rrs = append(rrs, rr)
		}
	}

	srv := new(dns.SRV)
	srv.Hdr = dns.RR_Header{Name: "_" + state.Proto() + "." + state.QName(), Rrtype: dns.TypeSRV, Class: state.QClass()}
	srv.Port = uint16(port)
	srv.Target = "."

	a.Answer = rrs
	a.Extra = []dns.RR{srv}
	state.SizeAndDo(a)
	w.WriteMsg(a)

	return 0, nil
}
