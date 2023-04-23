package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/coredns/coredns/plugin/atlas"
	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/coredns/coredns/plugin/atlas/record"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"
)

var (
	zoneImportOptions *ZoneImportOptions
)

type ZoneImportOptions struct {
	dsn    string
	file   string
	domain string
}

func NewZoneImportOptions() *ZoneImportOptions {
	return &ZoneImportOptions{}
}

func init() {
	zoneImportOptions = NewZoneImportOptions()

	zoneImportCmd := &cobra.Command{
		Use:          "zoneImport",
		Short:        "import a zone from a zone file",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := zoneImportOptions.Complete(c, args); err != nil {
				return err
			}
			if err := zoneImportOptions.Validate(); err != nil {
				return err
			}
			if err := zoneImportOptions.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	importZoneFlags := zoneImportCmd.Flags()
	importZoneFlags.StringVarP(&zoneImportOptions.domain, "domain", "d", "", "import directory")
	importZoneFlags.StringVarP(&zoneImportOptions.file, "file", "f", "", "zone file name to import")

	rootCmd.AddCommand(zoneImportCmd)
}

func (o *ZoneImportOptions) Complete(cmd *cobra.Command, args []string) (err error) {
	o.dsn = cfg.db.DSN

	return nil
}

func (o *ZoneImportOptions) Validate() (err error) {
	if len(o.file) == 0 {
		return fmt.Errorf("no file found")
	}

	if len(o.domain) == 0 {
		return fmt.Errorf("expected domain")
	}

	return nil
}

func (o *ZoneImportOptions) Run() error {
	var client *ent.Client
	var err error

	client, err = atlas.OpenAtlasDB(o.dsn)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.Schema.Create(context.Background())
	if err != nil {
		return err
	}

	f, err := os.Open(o.file)
	if err != nil {
		return err
	}
	z := dns.NewZoneParser(f, o.domain, o.file)
	if z.Err() != nil {
		return z.Err()
	}

	z.SetIncludeAllowed(false)
	var zone string
	for rr, ok := z.Next(); ok; rr, ok = z.Next() {
		switch t := rr.(type) {
		case *dns.SOA:
			zone = t.Hdr.Name
			if err := o.importSOA(client, t); err != nil {
				return err
			}
		case *dns.NS:
			rec, err := record.NewNS(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.A:
			rec, err := record.NewA(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.AAAA:
			rec, err := record.NewAAAA(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CNAME:
			rec, err := record.NewCNAME(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.PTR:
			rec, err := record.NewPTR(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.HINFO:
			rec, err := record.NewHINFO(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TXT:
			rec, err := record.NewTXT(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.MX:
			rec, err := record.NewMX(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.AFSDB:
			rec, err := record.NewAFSDB(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.RP:
			rec, err := record.NewRP(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.LOC:
			rec, err := record.NewLOC(t).Marshal()

			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SRV:
			rec, err := record.NewSRV(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DS:
			rec, err := record.NewDS(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TLSA:
			rec, err := record.NewTLSA(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CAA:
			rec, err := record.NewCAA(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DNAME:
			rec, err := record.NewDNAME(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.NAPTR:
			rec, err := record.NewNAPTR(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CSYNC:
			rec, err := record.NewCSYNC(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TKEY:
			rec, err := record.NewTKEY(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.ZONEMD:
			rec, err := record.NewZONEMD(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DHCID:
			rec, err := record.NewDHCID(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.HIP:
			rec, err := record.NewHIP(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SMIMEA:
			rec, err := record.NewSMIMEA(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.OPENPGPKEY:
			rec, err := record.NewOPENPGPKEY(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SSHFP:
			rec, err := record.NewSSHFP(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.KX:
			rec, err := record.NewKX(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.EUI48:
			rec, err := record.NewEUI48(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.EUI64:
			rec, err := record.NewEUI64(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.URI:
			rec, err := record.NewURI(t).Marshal()
			if err != nil {
				return err
			}
			if err := importRecord(client, zone, t, rec); err != nil {
				return err
			}

		}
	}

	return nil
}

func (o *ZoneImportOptions) importSOA(client *ent.Client, soa *dns.SOA) error {
	ctx := context.Background()
	count, err := client.Debug().DnsZone.Query().Where(dnszone.NameEQ(soa.Hdr.Name)).Count(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		fmt.Printf("zone already exists: %v\n", count)
		return nil
	}

	_, err = client.Debug().DnsZone.Create().
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

type DnsRecordResource interface {
	Header() *dns.RR_Header
}

func importRecord[K DnsRecordResource](client *ent.Client, zone string, i K, zone_record string) error {

	ctx := context.Background()
	header := i.Header()

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
