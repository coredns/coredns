package utils

import (
	"context"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/miekg/dns"
)

type DnsRecordResource interface {
	Header() *dns.RR_Header
}

func ImportRecord[K DnsRecordResource](client *ent.Client, zone string, i K, zone_record string) error {

	ctx := context.Background()
	header := i.Header()

	// if you dont want sql output strings, remove Debug().
	zoneId, err := client.Debug().DnsZone.Query().Where(
		dnszone.And(
			dnszone.NameEQ(zone),
		),
	).FirstID(ctx)
	if err != nil {
		return err
	}

	_, err = client.Debug().DnsRR.Create().
		SetName(header.Name).
		SetRrtype(header.Rrtype).
		SetRrdata(zone_record).
		SetClass(header.Class).
		SetTTL(header.Ttl).
		SetZoneID(zoneId).
		Save(ctx)

	return err
}
