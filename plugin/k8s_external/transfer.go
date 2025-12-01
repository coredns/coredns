package external

import (
	"context"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Transfer implements transfer.Transferer
func (e *External) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	z := plugin.Zones(e.Zones).Matches(zone)
	if z != zone {
		return nil, transfer.ErrNotAuthoritative
	}

	ctx := context.Background()
	ch := make(chan []dns.RR, 2)
	if zone == "." {
		zone = ""
	}
	state := request.Request{Zone: zone}

	// SOA
	soa := e.soa(state)
	ch <- []dns.RR{soa}
	if serial != 0 && serial >= soa.Serial {
		close(ch)
		return ch, nil
	}

	go func() {
		// Add NS
		nsName := "ns1." + e.apex + "." + zone
		nsHdr := dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Ttl: e.ttl, Class: dns.ClassINET}
		ch <- []dns.RR{&dns.NS{Hdr: nsHdr, Ns: nsName}}

		// Add Nameserver A/AAAA records
		nsRecords := e.externalAddrFunc(state, e.headless)
		for i := range nsRecords {
			// externalAddrFunc returns incomplete header names, correct here
			nsRecords[i].Header().Name = nsName
			nsRecords[i].Header().Ttl = e.ttl
			ch <- []dns.RR{nsRecords[i]}
		}

		svcs, headlessSvcs := e.externalServicesFunc(zone, e.headless)
		srvSeen := make(map[string]struct{})

		for i := range svcs {
			name := msg.Domain(svcs[i].Key)

			if svcs[i].TargetStrip == 0 {
				// Add Service A/AAAA records
				s := request.Request{Req: &dns.Msg{Question: []dns.Question{{Name: name}}}}
				as, _ := e.a(ctx, []msg.Service{svcs[i]}, s)
				if len(as) > 0 {
					ch <- as
				}
				aaaas, _ := e.aaaa(ctx, []msg.Service{svcs[i]}, s)
				if len(aaaas) > 0 {
					ch <- aaaas
				}
				// Add bare SRV record, ensuring uniqueness
				recs, _ := e.srv(ctx, []msg.Service{svcs[i]}, s)
				for _, srv := range recs {
					if !nameSeen(srvSeen, srv) {
						ch <- []dns.RR{srv}
					}
				}
				continue
			}
			// Add full SRV record, ensuring uniqueness
			s := request.Request{Req: &dns.Msg{Question: []dns.Question{{Name: name}}}}
			recs, _ := e.srv(ctx, []msg.Service{svcs[i]}, s)
			for _, srv := range recs {
				if !nameSeen(srvSeen, srv) {
					ch <- []dns.RR{srv}
				}
			}
		}
		for key, svcs := range headlessSvcs {
			// we have to strip the leading key because it's either port.protocol or endpoint
			name := msg.Domain(key[:strings.LastIndex(key, "/")])
			switchKey := key[strings.LastIndex(key, "/")+1:]
			switch switchKey {
			case kubernetes.Endpoint:
				// headless.namespace.example.com records
				s := request.Request{Req: &dns.Msg{Question: []dns.Question{{Name: name}}}}
				as, _ := e.a(ctx, svcs, s)
				if len(as) > 0 {
					ch <- as
				}
				aaaas, _ := e.aaaa(ctx, svcs, s)
				if len(aaaas) > 0 {
					ch <- aaaas
				}
				// Add bare SRV record, ensuring uniqueness
				recs, _ := e.srv(ctx, svcs, s)
				ch <- recs
				for _, srv := range recs {
					ch <- []dns.RR{srv}
				}

				for i := range svcs {
					// endpoint.headless.namespace.example.com record
					s := request.Request{Req: &dns.Msg{Question: []dns.Question{{Name: msg.Domain(svcs[i].Key)}}}}

					as, _ := e.a(ctx, []msg.Service{svcs[i]}, s)
					if len(as) > 0 {
						ch <- as
					}
					aaaas, _ := e.aaaa(ctx, []msg.Service{svcs[i]}, s)
					if len(aaaas) > 0 {
						ch <- aaaas
					}
					// Add bare SRV record, ensuring uniqueness
					recs, _ := e.srv(ctx, []msg.Service{svcs[i]}, s)
					ch <- recs
					for _, srv := range recs {
						ch <- []dns.RR{srv}
					}
				}

			case kubernetes.PortProtocol:
				s := request.Request{Req: &dns.Msg{Question: []dns.Question{{Name: name}}}}
				recs, _ := e.srv(ctx, svcs, s)
				ch <- recs
			}
		}
		ch <- []dns.RR{soa}
		close(ch)
	}()

	return ch, nil
}

func nameSeen(namesSeen map[string]struct{}, rr dns.RR) bool {
	if _, duplicate := namesSeen[rr.Header().Name]; duplicate {
		return true
	}
	namesSeen[rr.Header().Name] = struct{}{}
	return false
}
