// Code generated by coredns Atlas plugin; DO NOT EDIT.

package record

import (
	"encoding/json"
	"fmt"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/miekg/dns"
)

// Marshal CNAME RR and return json string and error if any
func (rec CNAME) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewCNAME creates a record.CNAME from *dns.CNAME
func NewCNAME(rec *dns.CNAME) CNAME {
	return CNAME{
		Target: rec.Target,
	}
}

// Marshal HINFO RR and return json string and error if any
func (rec HINFO) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewHINFO creates a record.HINFO from *dns.HINFO
func NewHINFO(rec *dns.HINFO) HINFO {
	return HINFO{
		Cpu: rec.Cpu,
		Os:  rec.Os,
	}
}

// Marshal MX RR and return json string and error if any
func (rec MX) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewMX creates a record.MX from *dns.MX
func NewMX(rec *dns.MX) MX {
	return MX{
		Preference: rec.Preference,
		Mx:         rec.Mx,
	}
}

// Marshal AFSDB RR and return json string and error if any
func (rec AFSDB) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewAFSDB creates a record.AFSDB from *dns.AFSDB
func NewAFSDB(rec *dns.AFSDB) AFSDB {
	return AFSDB{
		Subtype:  rec.Subtype,
		Hostname: rec.Hostname,
	}
}

// Marshal NS RR and return json string and error if any
func (rec NS) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewNS creates a record.NS from *dns.NS
func NewNS(rec *dns.NS) NS {
	return NS{
		Ns: rec.Ns,
	}
}

// Marshal PTR RR and return json string and error if any
func (rec PTR) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewPTR creates a record.PTR from *dns.PTR
func NewPTR(rec *dns.PTR) PTR {
	return PTR{
		Ptr: rec.Ptr,
	}
}

// Marshal RP RR and return json string and error if any
func (rec RP) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewRP creates a record.RP from *dns.RP
func NewRP(rec *dns.RP) RP {
	return RP{
		Mbox: rec.Mbox,
		Txt:  rec.Txt,
	}
}

// Marshal TXT RR and return json string and error if any
func (rec TXT) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewTXT creates a record.TXT from *dns.TXT
func NewTXT(rec *dns.TXT) TXT {
	return TXT{
		Txt: rec.Txt,
	}
}

// Marshal SRV RR and return json string and error if any
func (rec SRV) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewSRV creates a record.SRV from *dns.SRV
func NewSRV(rec *dns.SRV) SRV {
	return SRV{
		Priority: rec.Priority,
		Weight:   rec.Weight,
		Port:     rec.Port,
		Target:   rec.Target,
	}
}

// Marshal NAPTR RR and return json string and error if any
func (rec NAPTR) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewNAPTR creates a record.NAPTR from *dns.NAPTR
func NewNAPTR(rec *dns.NAPTR) NAPTR {
	return NAPTR{
		Order:       rec.Order,
		Preference:  rec.Preference,
		Flags:       rec.Flags,
		Service:     rec.Service,
		Regexp:      rec.Regexp,
		Replacement: rec.Replacement,
	}
}

// Marshal CERT RR and return json string and error if any
func (rec CERT) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewCERT creates a record.CERT from *dns.CERT
func NewCERT(rec *dns.CERT) CERT {
	return CERT{
		Type:        rec.Type,
		KeyTag:      rec.KeyTag,
		Algorithm:   rec.Algorithm,
		Certificate: rec.Certificate,
	}
}

// Marshal DNAME RR and return json string and error if any
func (rec DNAME) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewDNAME creates a record.DNAME from *dns.DNAME
func NewDNAME(rec *dns.DNAME) DNAME {
	return DNAME{
		Target: rec.Target,
	}
}

// Marshal A RR and return json string and error if any
func (rec A) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewA creates a record.A from *dns.A
func NewA(rec *dns.A) A {
	return A{
		A: rec.A,
	}
}

// Marshal AAAA RR and return json string and error if any
func (rec AAAA) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewAAAA creates a record.AAAA from *dns.AAAA
func NewAAAA(rec *dns.AAAA) AAAA {
	return AAAA{
		AAAA: rec.AAAA,
	}
}

// Marshal LOC RR and return json string and error if any
func (rec LOC) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewLOC creates a record.LOC from *dns.LOC
func NewLOC(rec *dns.LOC) LOC {
	return LOC{
		Version:   rec.Version,
		Size:      rec.Size,
		HorizPre:  rec.HorizPre,
		VertPre:   rec.VertPre,
		Latitude:  rec.Latitude,
		Longitude: rec.Longitude,
		Altitude:  rec.Altitude,
	}
}

// Marshal RRSIG RR and return json string and error if any
func (rec RRSIG) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewRRSIG creates a record.RRSIG from *dns.RRSIG
func NewRRSIG(rec *dns.RRSIG) RRSIG {
	return RRSIG{
		TypeCovered: rec.TypeCovered,
		Algorithm:   rec.Algorithm,
		Labels:      rec.Labels,
		OrigTtl:     rec.OrigTtl,
		Expiration:  rec.Expiration,
		Inception:   rec.Inception,
		KeyTag:      rec.KeyTag,
		SignerName:  rec.SignerName,
		Signature:   rec.Signature,
	}
}

// Marshal NSEC RR and return json string and error if any
func (rec NSEC) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewNSEC creates a record.NSEC from *dns.NSEC
func NewNSEC(rec *dns.NSEC) NSEC {
	return NSEC{
		NextDomain: rec.NextDomain,
		TypeBitMap: rec.TypeBitMap,
	}
}

// Marshal DS RR and return json string and error if any
func (rec DS) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewDS creates a record.DS from *dns.DS
func NewDS(rec *dns.DS) DS {
	return DS{
		KeyTag:     rec.KeyTag,
		Algorithm:  rec.Algorithm,
		DigestType: rec.DigestType,
		Digest:     rec.Digest,
	}
}

// Marshal KX RR and return json string and error if any
func (rec KX) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewKX creates a record.KX from *dns.KX
func NewKX(rec *dns.KX) KX {
	return KX{
		Preference: rec.Preference,
		Exchanger:  rec.Exchanger,
	}
}

// Marshal TA RR and return json string and error if any
func (rec TA) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewTA creates a record.TA from *dns.TA
func NewTA(rec *dns.TA) TA {
	return TA{
		KeyTag:     rec.KeyTag,
		Algorithm:  rec.Algorithm,
		DigestType: rec.DigestType,
		Digest:     rec.Digest,
	}
}

// Marshal SSHFP RR and return json string and error if any
func (rec SSHFP) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewSSHFP creates a record.SSHFP from *dns.SSHFP
func NewSSHFP(rec *dns.SSHFP) SSHFP {
	return SSHFP{
		Algorithm:   rec.Algorithm,
		Type:        rec.Type,
		FingerPrint: rec.FingerPrint,
	}
}

// Marshal DNSKEY RR and return json string and error if any
func (rec DNSKEY) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewDNSKEY creates a record.DNSKEY from *dns.DNSKEY
func NewDNSKEY(rec *dns.DNSKEY) DNSKEY {
	return DNSKEY{
		Flags:     rec.Flags,
		Protocol:  rec.Protocol,
		Algorithm: rec.Algorithm,
		PublicKey: rec.PublicKey,
	}
}

// Marshal IPSECKEY RR and return json string and error if any
func (rec IPSECKEY) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewIPSECKEY creates a record.IPSECKEY from *dns.IPSECKEY
func NewIPSECKEY(rec *dns.IPSECKEY) IPSECKEY {
	return IPSECKEY{
		Precedence:  rec.Precedence,
		GatewayType: rec.GatewayType,
		Algorithm:   rec.Algorithm,
		GatewayAddr: rec.GatewayAddr,
		GatewayHost: rec.GatewayHost,
		PublicKey:   rec.PublicKey,
	}
}

// Marshal NSEC3 RR and return json string and error if any
func (rec NSEC3) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewNSEC3 creates a record.NSEC3 from *dns.NSEC3
func NewNSEC3(rec *dns.NSEC3) NSEC3 {
	return NSEC3{
		Hash:       rec.Hash,
		Flags:      rec.Flags,
		Iterations: rec.Iterations,
		SaltLength: rec.SaltLength,
		Salt:       rec.Salt,
		HashLength: rec.HashLength,
		NextDomain: rec.NextDomain,
		TypeBitMap: rec.TypeBitMap,
	}
}

// Marshal NSEC3PARAM RR and return json string and error if any
func (rec NSEC3PARAM) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewNSEC3PARAM creates a record.NSEC3PARAM from *dns.NSEC3PARAM
func NewNSEC3PARAM(rec *dns.NSEC3PARAM) NSEC3PARAM {
	return NSEC3PARAM{
		Hash:       rec.Hash,
		Flags:      rec.Flags,
		Iterations: rec.Iterations,
		SaltLength: rec.SaltLength,
		Salt:       rec.Salt,
	}
}

// Marshal TKEY RR and return json string and error if any
func (rec TKEY) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewTKEY creates a record.TKEY from *dns.TKEY
func NewTKEY(rec *dns.TKEY) TKEY {
	return TKEY{
		Algorithm:  rec.Algorithm,
		Inception:  rec.Inception,
		Expiration: rec.Expiration,
		Mode:       rec.Mode,
		Error:      rec.Error,
		KeySize:    rec.KeySize,
		Key:        rec.Key,
		OtherLen:   rec.OtherLen,
		OtherData:  rec.OtherData,
	}
}

// Marshal URI RR and return json string and error if any
func (rec URI) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewURI creates a record.URI from *dns.URI
func NewURI(rec *dns.URI) URI {
	return URI{
		Priority: rec.Priority,
		Weight:   rec.Weight,
		Target:   rec.Target,
	}
}

// Marshal DHCID RR and return json string and error if any
func (rec DHCID) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewDHCID creates a record.DHCID from *dns.DHCID
func NewDHCID(rec *dns.DHCID) DHCID {
	return DHCID{
		Digest: rec.Digest,
	}
}

// Marshal TLSA RR and return json string and error if any
func (rec TLSA) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewTLSA creates a record.TLSA from *dns.TLSA
func NewTLSA(rec *dns.TLSA) TLSA {
	return TLSA{
		Usage:        rec.Usage,
		Selector:     rec.Selector,
		MatchingType: rec.MatchingType,
		Certificate:  rec.Certificate,
	}
}

// Marshal SMIMEA RR and return json string and error if any
func (rec SMIMEA) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewSMIMEA creates a record.SMIMEA from *dns.SMIMEA
func NewSMIMEA(rec *dns.SMIMEA) SMIMEA {
	return SMIMEA{
		Usage:        rec.Usage,
		Selector:     rec.Selector,
		MatchingType: rec.MatchingType,
		Certificate:  rec.Certificate,
	}
}

// Marshal HIP RR and return json string and error if any
func (rec HIP) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewHIP creates a record.HIP from *dns.HIP
func NewHIP(rec *dns.HIP) HIP {
	return HIP{
		HitLength:          rec.HitLength,
		PublicKeyAlgorithm: rec.PublicKeyAlgorithm,
		PublicKeyLength:    rec.PublicKeyLength,
		Hit:                rec.Hit,
		PublicKey:          rec.PublicKey,
		RendezvousServers:  rec.RendezvousServers,
	}
}

// Marshal EUI48 RR and return json string and error if any
func (rec EUI48) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewEUI48 creates a record.EUI48 from *dns.EUI48
func NewEUI48(rec *dns.EUI48) EUI48 {
	return EUI48{
		Address: rec.Address,
	}
}

// Marshal EUI64 RR and return json string and error if any
func (rec EUI64) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewEUI64 creates a record.EUI64 from *dns.EUI64
func NewEUI64(rec *dns.EUI64) EUI64 {
	return EUI64{
		Address: rec.Address,
	}
}

// Marshal CAA RR and return json string and error if any
func (rec CAA) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewCAA creates a record.CAA from *dns.CAA
func NewCAA(rec *dns.CAA) CAA {
	return CAA{
		Flag:  rec.Flag,
		Tag:   rec.Tag,
		Value: rec.Value,
	}
}

// Marshal OPENPGPKEY RR and return json string and error if any
func (rec OPENPGPKEY) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewOPENPGPKEY creates a record.OPENPGPKEY from *dns.OPENPGPKEY
func NewOPENPGPKEY(rec *dns.OPENPGPKEY) OPENPGPKEY {
	return OPENPGPKEY{
		PublicKey: rec.PublicKey,
	}
}

// Marshal CSYNC RR and return json string and error if any
func (rec CSYNC) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewCSYNC creates a record.CSYNC from *dns.CSYNC
func NewCSYNC(rec *dns.CSYNC) CSYNC {
	return CSYNC{
		Serial:     rec.Serial,
		Flags:      rec.Flags,
		TypeBitMap: rec.TypeBitMap,
	}
}

// Marshal ZONEMD RR and return json string and error if any
func (rec ZONEMD) Marshal() (s string, e error) {
	var m []byte
	if m, e = json.Marshal(rec); e != nil {
		return
	}
	return string(m), nil
}

// NewZONEMD creates a record.ZONEMD from *dns.ZONEMD
func NewZONEMD(rec *dns.ZONEMD) ZONEMD {
	return ZONEMD{
		Serial: rec.Serial,
		Scheme: rec.Scheme,
		Hash:   rec.Hash,
		Digest: rec.Digest,
	}
}

// From returns a dns.RR from ent.DnsRR
func From(rec *ent.DnsRR) (dns.RR, error) {
	if rec == nil {
		return nil, fmt.Errorf("unexpected DnsRR record")
	}

	header, err := GetRRHeaderFromDnsRR(rec)
	if err != nil {
		return nil, err
	}

	switch rec.Rrtype {

	case dns.TypeCNAME:
		var recCNAME CNAME
		if err := json.Unmarshal([]byte(rec.Rrdata), &recCNAME); err != nil {
			return nil, err
		}

		cname := dns.CNAME{
			Hdr:    *header,
			Target: recCNAME.Target,
		}

		return &cname, nil

	case dns.TypeHINFO:
		var recHINFO HINFO
		if err := json.Unmarshal([]byte(rec.Rrdata), &recHINFO); err != nil {
			return nil, err
		}

		hinfo := dns.HINFO{
			Hdr: *header,
			Cpu: recHINFO.Cpu,
			Os:  recHINFO.Os,
		}

		return &hinfo, nil

	case dns.TypeMX:
		var recMX MX
		if err := json.Unmarshal([]byte(rec.Rrdata), &recMX); err != nil {
			return nil, err
		}

		mx := dns.MX{
			Hdr:        *header,
			Preference: recMX.Preference,
			Mx:         recMX.Mx,
		}

		return &mx, nil

	case dns.TypeAFSDB:
		var recAFSDB AFSDB
		if err := json.Unmarshal([]byte(rec.Rrdata), &recAFSDB); err != nil {
			return nil, err
		}

		afsdb := dns.AFSDB{
			Hdr:      *header,
			Subtype:  recAFSDB.Subtype,
			Hostname: recAFSDB.Hostname,
		}

		return &afsdb, nil

	case dns.TypeNS:
		var recNS NS
		if err := json.Unmarshal([]byte(rec.Rrdata), &recNS); err != nil {
			return nil, err
		}

		ns := dns.NS{
			Hdr: *header,
			Ns:  recNS.Ns,
		}

		return &ns, nil

	case dns.TypePTR:
		var recPTR PTR
		if err := json.Unmarshal([]byte(rec.Rrdata), &recPTR); err != nil {
			return nil, err
		}

		ptr := dns.PTR{
			Hdr: *header,
			Ptr: recPTR.Ptr,
		}

		return &ptr, nil

	case dns.TypeRP:
		var recRP RP
		if err := json.Unmarshal([]byte(rec.Rrdata), &recRP); err != nil {
			return nil, err
		}

		rp := dns.RP{
			Hdr:  *header,
			Mbox: recRP.Mbox,
			Txt:  recRP.Txt,
		}

		return &rp, nil

	case dns.TypeTXT:
		var recTXT TXT
		if err := json.Unmarshal([]byte(rec.Rrdata), &recTXT); err != nil {
			return nil, err
		}

		txt := dns.TXT{
			Hdr: *header,
			Txt: recTXT.Txt,
		}

		return &txt, nil

	case dns.TypeSRV:
		var recSRV SRV
		if err := json.Unmarshal([]byte(rec.Rrdata), &recSRV); err != nil {
			return nil, err
		}

		srv := dns.SRV{
			Hdr:      *header,
			Priority: recSRV.Priority,
			Weight:   recSRV.Weight,
			Port:     recSRV.Port,
			Target:   recSRV.Target,
		}

		return &srv, nil

	case dns.TypeNAPTR:
		var recNAPTR NAPTR
		if err := json.Unmarshal([]byte(rec.Rrdata), &recNAPTR); err != nil {
			return nil, err
		}

		naptr := dns.NAPTR{
			Hdr:         *header,
			Order:       recNAPTR.Order,
			Preference:  recNAPTR.Preference,
			Flags:       recNAPTR.Flags,
			Service:     recNAPTR.Service,
			Regexp:      recNAPTR.Regexp,
			Replacement: recNAPTR.Replacement,
		}

		return &naptr, nil

	case dns.TypeCERT:
		var recCERT CERT
		if err := json.Unmarshal([]byte(rec.Rrdata), &recCERT); err != nil {
			return nil, err
		}

		cert := dns.CERT{
			Hdr:         *header,
			Type:        recCERT.Type,
			KeyTag:      recCERT.KeyTag,
			Algorithm:   recCERT.Algorithm,
			Certificate: recCERT.Certificate,
		}

		return &cert, nil

	case dns.TypeDNAME:
		var recDNAME DNAME
		if err := json.Unmarshal([]byte(rec.Rrdata), &recDNAME); err != nil {
			return nil, err
		}

		dname := dns.DNAME{
			Hdr:    *header,
			Target: recDNAME.Target,
		}

		return &dname, nil

	case dns.TypeA:
		var recA A
		if err := json.Unmarshal([]byte(rec.Rrdata), &recA); err != nil {
			return nil, err
		}

		a := dns.A{
			Hdr: *header,
			A:   recA.A,
		}

		return &a, nil

	case dns.TypeAAAA:
		var recAAAA AAAA
		if err := json.Unmarshal([]byte(rec.Rrdata), &recAAAA); err != nil {
			return nil, err
		}

		aaaa := dns.AAAA{
			Hdr:  *header,
			AAAA: recAAAA.AAAA,
		}

		return &aaaa, nil

	case dns.TypeLOC:
		var recLOC LOC
		if err := json.Unmarshal([]byte(rec.Rrdata), &recLOC); err != nil {
			return nil, err
		}

		loc := dns.LOC{
			Hdr:       *header,
			Version:   recLOC.Version,
			Size:      recLOC.Size,
			HorizPre:  recLOC.HorizPre,
			VertPre:   recLOC.VertPre,
			Latitude:  recLOC.Latitude,
			Longitude: recLOC.Longitude,
			Altitude:  recLOC.Altitude,
		}

		return &loc, nil

	case dns.TypeRRSIG:
		var recRRSIG RRSIG
		if err := json.Unmarshal([]byte(rec.Rrdata), &recRRSIG); err != nil {
			return nil, err
		}

		rrsig := dns.RRSIG{
			Hdr:         *header,
			TypeCovered: recRRSIG.TypeCovered,
			Algorithm:   recRRSIG.Algorithm,
			Labels:      recRRSIG.Labels,
			OrigTtl:     recRRSIG.OrigTtl,
			Expiration:  recRRSIG.Expiration,
			Inception:   recRRSIG.Inception,
			KeyTag:      recRRSIG.KeyTag,
			SignerName:  recRRSIG.SignerName,
			Signature:   recRRSIG.Signature,
		}

		return &rrsig, nil

	case dns.TypeNSEC:
		var recNSEC NSEC
		if err := json.Unmarshal([]byte(rec.Rrdata), &recNSEC); err != nil {
			return nil, err
		}

		nsec := dns.NSEC{
			Hdr:        *header,
			NextDomain: recNSEC.NextDomain,
			TypeBitMap: recNSEC.TypeBitMap,
		}

		return &nsec, nil

	case dns.TypeDS:
		var recDS DS
		if err := json.Unmarshal([]byte(rec.Rrdata), &recDS); err != nil {
			return nil, err
		}

		ds := dns.DS{
			Hdr:        *header,
			KeyTag:     recDS.KeyTag,
			Algorithm:  recDS.Algorithm,
			DigestType: recDS.DigestType,
			Digest:     recDS.Digest,
		}

		return &ds, nil

	case dns.TypeKX:
		var recKX KX
		if err := json.Unmarshal([]byte(rec.Rrdata), &recKX); err != nil {
			return nil, err
		}

		kx := dns.KX{
			Hdr:        *header,
			Preference: recKX.Preference,
			Exchanger:  recKX.Exchanger,
		}

		return &kx, nil

	case dns.TypeTA:
		var recTA TA
		if err := json.Unmarshal([]byte(rec.Rrdata), &recTA); err != nil {
			return nil, err
		}

		ta := dns.TA{
			Hdr:        *header,
			KeyTag:     recTA.KeyTag,
			Algorithm:  recTA.Algorithm,
			DigestType: recTA.DigestType,
			Digest:     recTA.Digest,
		}

		return &ta, nil

	case dns.TypeSSHFP:
		var recSSHFP SSHFP
		if err := json.Unmarshal([]byte(rec.Rrdata), &recSSHFP); err != nil {
			return nil, err
		}

		sshfp := dns.SSHFP{
			Hdr:         *header,
			Algorithm:   recSSHFP.Algorithm,
			Type:        recSSHFP.Type,
			FingerPrint: recSSHFP.FingerPrint,
		}

		return &sshfp, nil

	case dns.TypeDNSKEY:
		var recDNSKEY DNSKEY
		if err := json.Unmarshal([]byte(rec.Rrdata), &recDNSKEY); err != nil {
			return nil, err
		}

		dnskey := dns.DNSKEY{
			Hdr:       *header,
			Flags:     recDNSKEY.Flags,
			Protocol:  recDNSKEY.Protocol,
			Algorithm: recDNSKEY.Algorithm,
			PublicKey: recDNSKEY.PublicKey,
		}

		return &dnskey, nil

	case dns.TypeIPSECKEY:
		var recIPSECKEY IPSECKEY
		if err := json.Unmarshal([]byte(rec.Rrdata), &recIPSECKEY); err != nil {
			return nil, err
		}

		ipseckey := dns.IPSECKEY{
			Hdr:         *header,
			Precedence:  recIPSECKEY.Precedence,
			GatewayType: recIPSECKEY.GatewayType,
			Algorithm:   recIPSECKEY.Algorithm,
			GatewayAddr: recIPSECKEY.GatewayAddr,
			GatewayHost: recIPSECKEY.GatewayHost,
			PublicKey:   recIPSECKEY.PublicKey,
		}

		return &ipseckey, nil

	case dns.TypeNSEC3:
		var recNSEC3 NSEC3
		if err := json.Unmarshal([]byte(rec.Rrdata), &recNSEC3); err != nil {
			return nil, err
		}

		nsec3 := dns.NSEC3{
			Hdr:        *header,
			Hash:       recNSEC3.Hash,
			Flags:      recNSEC3.Flags,
			Iterations: recNSEC3.Iterations,
			SaltLength: recNSEC3.SaltLength,
			Salt:       recNSEC3.Salt,
			HashLength: recNSEC3.HashLength,
			NextDomain: recNSEC3.NextDomain,
			TypeBitMap: recNSEC3.TypeBitMap,
		}

		return &nsec3, nil

	case dns.TypeNSEC3PARAM:
		var recNSEC3PARAM NSEC3PARAM
		if err := json.Unmarshal([]byte(rec.Rrdata), &recNSEC3PARAM); err != nil {
			return nil, err
		}

		nsec3param := dns.NSEC3PARAM{
			Hdr:        *header,
			Hash:       recNSEC3PARAM.Hash,
			Flags:      recNSEC3PARAM.Flags,
			Iterations: recNSEC3PARAM.Iterations,
			SaltLength: recNSEC3PARAM.SaltLength,
			Salt:       recNSEC3PARAM.Salt,
		}

		return &nsec3param, nil

	case dns.TypeTKEY:
		var recTKEY TKEY
		if err := json.Unmarshal([]byte(rec.Rrdata), &recTKEY); err != nil {
			return nil, err
		}

		tkey := dns.TKEY{
			Hdr:        *header,
			Algorithm:  recTKEY.Algorithm,
			Inception:  recTKEY.Inception,
			Expiration: recTKEY.Expiration,
			Mode:       recTKEY.Mode,
			Error:      recTKEY.Error,
			KeySize:    recTKEY.KeySize,
			Key:        recTKEY.Key,
			OtherLen:   recTKEY.OtherLen,
			OtherData:  recTKEY.OtherData,
		}

		return &tkey, nil

	case dns.TypeURI:
		var recURI URI
		if err := json.Unmarshal([]byte(rec.Rrdata), &recURI); err != nil {
			return nil, err
		}

		uri := dns.URI{
			Hdr:      *header,
			Priority: recURI.Priority,
			Weight:   recURI.Weight,
			Target:   recURI.Target,
		}

		return &uri, nil

	case dns.TypeDHCID:
		var recDHCID DHCID
		if err := json.Unmarshal([]byte(rec.Rrdata), &recDHCID); err != nil {
			return nil, err
		}

		dhcid := dns.DHCID{
			Hdr:    *header,
			Digest: recDHCID.Digest,
		}

		return &dhcid, nil

	case dns.TypeTLSA:
		var recTLSA TLSA
		if err := json.Unmarshal([]byte(rec.Rrdata), &recTLSA); err != nil {
			return nil, err
		}

		tlsa := dns.TLSA{
			Hdr:          *header,
			Usage:        recTLSA.Usage,
			Selector:     recTLSA.Selector,
			MatchingType: recTLSA.MatchingType,
			Certificate:  recTLSA.Certificate,
		}

		return &tlsa, nil

	case dns.TypeSMIMEA:
		var recSMIMEA SMIMEA
		if err := json.Unmarshal([]byte(rec.Rrdata), &recSMIMEA); err != nil {
			return nil, err
		}

		smimea := dns.SMIMEA{
			Hdr:          *header,
			Usage:        recSMIMEA.Usage,
			Selector:     recSMIMEA.Selector,
			MatchingType: recSMIMEA.MatchingType,
			Certificate:  recSMIMEA.Certificate,
		}

		return &smimea, nil

	case dns.TypeHIP:
		var recHIP HIP
		if err := json.Unmarshal([]byte(rec.Rrdata), &recHIP); err != nil {
			return nil, err
		}

		hip := dns.HIP{
			Hdr:                *header,
			HitLength:          recHIP.HitLength,
			PublicKeyAlgorithm: recHIP.PublicKeyAlgorithm,
			PublicKeyLength:    recHIP.PublicKeyLength,
			Hit:                recHIP.Hit,
			PublicKey:          recHIP.PublicKey,
			RendezvousServers:  recHIP.RendezvousServers,
		}

		return &hip, nil

	case dns.TypeEUI48:
		var recEUI48 EUI48
		if err := json.Unmarshal([]byte(rec.Rrdata), &recEUI48); err != nil {
			return nil, err
		}

		eui48 := dns.EUI48{
			Hdr:     *header,
			Address: recEUI48.Address,
		}

		return &eui48, nil

	case dns.TypeEUI64:
		var recEUI64 EUI64
		if err := json.Unmarshal([]byte(rec.Rrdata), &recEUI64); err != nil {
			return nil, err
		}

		eui64 := dns.EUI64{
			Hdr:     *header,
			Address: recEUI64.Address,
		}

		return &eui64, nil

	case dns.TypeCAA:
		var recCAA CAA
		if err := json.Unmarshal([]byte(rec.Rrdata), &recCAA); err != nil {
			return nil, err
		}

		caa := dns.CAA{
			Hdr:   *header,
			Flag:  recCAA.Flag,
			Tag:   recCAA.Tag,
			Value: recCAA.Value,
		}

		return &caa, nil

	case dns.TypeOPENPGPKEY:
		var recOPENPGPKEY OPENPGPKEY
		if err := json.Unmarshal([]byte(rec.Rrdata), &recOPENPGPKEY); err != nil {
			return nil, err
		}

		openpgpkey := dns.OPENPGPKEY{
			Hdr:       *header,
			PublicKey: recOPENPGPKEY.PublicKey,
		}

		return &openpgpkey, nil

	case dns.TypeCSYNC:
		var recCSYNC CSYNC
		if err := json.Unmarshal([]byte(rec.Rrdata), &recCSYNC); err != nil {
			return nil, err
		}

		csync := dns.CSYNC{
			Hdr:        *header,
			Serial:     recCSYNC.Serial,
			Flags:      recCSYNC.Flags,
			TypeBitMap: recCSYNC.TypeBitMap,
		}

		return &csync, nil

	case dns.TypeZONEMD:
		var recZONEMD ZONEMD
		if err := json.Unmarshal([]byte(rec.Rrdata), &recZONEMD); err != nil {
			return nil, err
		}

		zonemd := dns.ZONEMD{
			Hdr:    *header,
			Serial: recZONEMD.Serial,
			Scheme: recZONEMD.Scheme,
			Hash:   recZONEMD.Hash,
			Digest: recZONEMD.Digest,
		}

		return &zonemd, nil

	default:
		return nil, fmt.Errorf("unknown dns.RR")
	}
}