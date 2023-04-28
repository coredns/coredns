package utils

import (
	"context"

	"github.com/coredns/coredns/plugin/sql/ent"
	"github.com/coredns/coredns/plugin/sql/ent/dnszone"
	"github.com/miekg/dns"
)

type DnsRecordResource interface {
	Header() *dns.RR_Header
}

// ImportRecord imports a zone, the RR_header and the zone_record into the database
// the zone_record has to be a json string marshalled with New<RR>().Marshal().
// The zone must exists.
func ImportRecord[K DnsRecordResource](client *ent.Client, zone string, i K, zone_record string) error {
	ctx := context.Background()
	header := i.Header()

	zoneId, err := client.DnsZone.
		Query().
		Where(
			dnszone.NameEQ(zone),
		).
		FirstID(ctx)
	if err != nil {
		return err
	}

	_, err = client.DnsRR.Create().
		SetName(header.Name).
		SetRrtype(header.Rrtype).
		SetRrdata(zone_record).
		SetClass(header.Class).
		SetTTL(header.Ttl).
		SetZoneID(zoneId).
		Save(ctx)

	return err
}
