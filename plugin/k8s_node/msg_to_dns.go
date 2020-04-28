package k8s_node

import (
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func (e *Node) a(services []msg.Service, state request.Request) (records []dns.RR) {
	dup := make(map[string]struct{})

	for _, s := range services {

		what, ip := s.HostType()

		switch what {
		case dns.TypeCNAME:
			// can't happen

		case dns.TypeA:
			if _, ok := dup[s.Host]; !ok {
				dup[s.Host] = struct{}{}
				rr := s.NewA(state.QName(), ip)
				rr.Hdr.Ttl = e.ttl
				records = append(records, rr)
			}

		case dns.TypeAAAA:
			// nada
		}
	}
	return records
}

func (e *Node) aaaa(services []msg.Service, state request.Request) (records []dns.RR) {
	dup := make(map[string]struct{})

	for _, s := range services {

		what, ip := s.HostType()

		switch what {
		case dns.TypeCNAME:
			// can't happen

		case dns.TypeA:
			// nada

		case dns.TypeAAAA:
			if _, ok := dup[s.Host]; !ok {
				dup[s.Host] = struct{}{}
				rr := s.NewAAAA(state.QName(), ip)
				rr.Hdr.Ttl = e.ttl
				records = append(records, rr)
			}
		}
	}
	return records
}
