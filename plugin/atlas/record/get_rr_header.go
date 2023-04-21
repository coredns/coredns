package record

import (
	"fmt"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/miekg/dns"
)

// GetRRHeaderFromDnsRR produces a `dns.RR_Header` from a database
// record `ent.DnsRR`
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
