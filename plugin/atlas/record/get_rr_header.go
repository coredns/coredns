package record

import (
	"fmt"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/miekg/dns"
)

func GetRRHeaderFromDnsRR(rec *ent.DnsRR) (*dns.RR_Header, error) {
	if rec == nil {
		return nil, fmt.Errorf("unexpected atlas resource record")
	}

	header := dns.RR_Header{
		Name:   rec.Name,
		Rrtype: rec.Rrtype,
		Class:  rec.Class,
		Ttl:    rec.TTL,
	}

	return &header, nil
}
