package etcd

import (
	"net"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/etcd/msg"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
)

// Remove!
// Options are extra options that can be specified for a lookup.
type Options struct {
	Debug string // This is a debug query. A query prefixed with debug.o-o
}

// MX returns MX records from etcd.
// If the Target is not a name but an IP address, a name is created on the fly.
func (e Etcd) MX(zone string, state request.Request, opt Options) (records, extra []dns.RR, debug []msg.Service, err error) {
	services, debug, err := e.records(state, false, opt)
	if err != nil {
		return nil, nil, debug, err
	}

	lookup := make(map[string]bool)
	for _, serv := range services {
		if !serv.Mail {
			continue
		}
		ip := net.ParseIP(serv.Host)
		switch {
		case ip == nil:
			mx := serv.NewMX(state.QName())
			records = append(records, mx)
			if _, ok := lookup[mx.Mx]; ok {
				break
			}

			lookup[mx.Mx] = true

			if !dns.IsSubDomain(zone, mx.Mx) {
				m1, e1 := e.Proxy.Lookup(state, mx.Mx, dns.TypeA)
				if e1 == nil {
					extra = append(extra, m1.Answer...)
				} else {
					debugMsg := msg.Service{Key: msg.Path(mx.Mx, e.PathPrefix), Host: mx.Mx, Text: " IN A: " + e1.Error()}
					debug = append(debug, debugMsg)
				}
				m1, e1 = e.Proxy.Lookup(state, mx.Mx, dns.TypeAAAA)
				if e1 == nil {
					// If we have seen CNAME's we *assume* that they are already added.
					for _, a := range m1.Answer {
						if _, ok := a.(*dns.CNAME); !ok {
							extra = append(extra, a)
						}
					}
				} else {
					debugMsg := msg.Service{Key: msg.Path(mx.Mx, e.PathPrefix), Host: mx.Mx, Text: " IN AAAA: " + e1.Error()}
					debug = append(debug, debugMsg)
				}
				break
			}
			// Internal name
			state1 := state.NewWithQuestion(mx.Mx, dns.TypeA)
			addr, debugAddr, e1 := middleware.A(e, zone, state1, nil, middleware.Options(opt))
			if e1 == nil {
				extra = append(extra, addr...)
				debug = append(debug, debugAddr...)
			}
			// e.AAAA as well
		case ip.To4() != nil:
			serv.Host = msg.Domain(serv.Key)
			records = append(records, serv.NewMX(state.QName()))
			extra = append(extra, serv.NewA(serv.Host, ip.To4()))
		case ip.To4() == nil:
			serv.Host = msg.Domain(serv.Key)
			records = append(records, serv.NewMX(state.QName()))
			extra = append(extra, serv.NewAAAA(serv.Host, ip.To16()))
		}
	}
	return records, extra, debug, nil
}

// CNAME returns CNAME records from etcd or an error.
func (e Etcd) CNAME(zone string, state request.Request, opt Options) (records []dns.RR, debug []msg.Service, err error) {
	services, debug, err := e.records(state, true, opt)
	if err != nil {
		return nil, debug, err
	}

	if len(services) > 0 {
		serv := services[0]
		if ip := net.ParseIP(serv.Host); ip == nil {
			records = append(records, serv.NewCNAME(state.QName(), serv.Host))
		}
	}
	return records, debug, nil
}

// TXT returns TXT records from etcd or an error.
func (e Etcd) TXT(zone string, state request.Request, opt Options) (records []dns.RR, debug []msg.Service, err error) {
	services, debug, err := e.records(state, false, opt)
	if err != nil {
		return nil, debug, err
	}

	for _, serv := range services {
		if serv.Text == "" {
			continue
		}
		records = append(records, serv.NewTXT(state.QName()))
	}
	return records, debug, nil
}
