package utils

import (
	"context"
	"fmt"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/miekg/dns"
)

// ImportSOA imports a soa record into the database
func ImportSOA(client *ent.Client, soa *dns.SOA) error {
	ctx := context.Background()
	count, err := client.DnsZone.Query().Where(dnszone.NameEQ(soa.Hdr.Name)).Count(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		fmt.Printf("zone already exists: %v\n", soa.Hdr.Name)
		return nil
	}

	_, err = client.DnsZone.
		Create().
		SetName(soa.Hdr.Name).
		SetRrtype(soa.Hdr.Rrtype).
		SetClass(soa.Hdr.Class).
		SetTTL(soa.Hdr.Ttl).
		SetExpire(soa.Expire).
		SetNs(soa.Ns).
		SetMbox(soa.Mbox).
		SetSerial(soa.Serial).
		SetRefresh(soa.Refresh).
		SetRetry(soa.Retry).
		SetExpire(soa.Expire).
		SetMinttl(soa.Minttl).
		Save(context.Background())
	return err
}
