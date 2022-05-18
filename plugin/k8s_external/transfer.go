package external

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
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
		nsRecords := e.externalAddrFunc(state)
		for i := range nsRecords {
			// externalAddrFunc returns incomplete header names, correct here
			nsRecords[i].Header().Name = nsName
			nsRecords[i].Header().Ttl = e.ttl
			ch <- []dns.RR{nsRecords[i]}
		}

		svcs := e.externalServicesFunc(zone)
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
