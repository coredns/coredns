package utils

import (
	"context"
	"io"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/record"
	"github.com/miekg/dns"
)

// ImportZone imports an RFC 1035-compliant zone file into the database
func ImportZone(ctx context.Context, client *ent.Client, reader io.Reader, origin string, file string) error {
	z := dns.NewZoneParser(reader, origin, file)
	if z.Err() != nil {
		return z.Err()
	}

	// we dont use includes and including files is maybe a security risk
	z.SetIncludeAllowed(false)

	var zone string
	for rr, ok := z.Next(); ok; rr, ok = z.Next() {
		switch t := rr.(type) {
		case *dns.SOA:
			zone = t.Hdr.Name
			if err := ImportSOA(client, t); err != nil {
				return err
			}
		case *dns.NS:
			rec, err := record.NewNS(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.A:
			rec, err := record.NewA(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.AAAA:
			rec, err := record.NewAAAA(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CNAME:
			rec, err := record.NewCNAME(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.PTR:
			rec, err := record.NewPTR(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.HINFO:
			rec, err := record.NewHINFO(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TXT:
			rec, err := record.NewTXT(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.MX:
			rec, err := record.NewMX(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.AFSDB:
			rec, err := record.NewAFSDB(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.RP:
			rec, err := record.NewRP(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.LOC:
			rec, err := record.NewLOC(t).Marshal()

			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SRV:
			rec, err := record.NewSRV(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DS:
			rec, err := record.NewDS(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TLSA:
			rec, err := record.NewTLSA(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CAA:
			rec, err := record.NewCAA(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DNAME:
			rec, err := record.NewDNAME(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.NAPTR:
			rec, err := record.NewNAPTR(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.CSYNC:
			rec, err := record.NewCSYNC(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.TKEY:
			rec, err := record.NewTKEY(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.ZONEMD:
			rec, err := record.NewZONEMD(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.DHCID:
			rec, err := record.NewDHCID(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.HIP:
			rec, err := record.NewHIP(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SMIMEA:
			rec, err := record.NewSMIMEA(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.OPENPGPKEY:
			rec, err := record.NewOPENPGPKEY(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.SSHFP:
			rec, err := record.NewSSHFP(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.KX:
			rec, err := record.NewKX(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.EUI48:
			rec, err := record.NewEUI48(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.EUI64:
			rec, err := record.NewEUI64(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		case *dns.URI:
			rec, err := record.NewURI(t).Marshal()
			if err != nil {
				return err
			}
			if err := ImportRecord(client, zone, t, rec); err != nil {
				return err
			}
		}
	}

	return nil
}