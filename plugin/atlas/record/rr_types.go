package record

import (
	"net"
)

//go:generate go run generate.go

// CNAME canonical Name RR
type CNAME struct {
	Target string `json:"target"`
}

// HINFO RR. See RFC 1034.
type HINFO struct {
	Cpu string `json:"cpu"`
	Os  string `json:"os"`
}

// MB RR. See RFC 1035.
// type MB struct {
// 	Mb string `json:"mb"`
// }

// MG RR
// type MG struct {
// 	Mg string `json:"mg"`
// }

// MINFO RR. See RFC 1035.
// type MINFO struct {
// 	Rmail string `json:"rmail"`
// 	Email string `json:"email"`
// }

// MR RR. See RFC 1035.
// type MR struct {
// 	Mr string `json:"mr"`
// }

// MF RR. See RFC 1035.
// type MF struct {
// 	Mf string `json:"mf"`
// }

// MD RR. See RFC 1035.
// type MD struct {
// 	Md string `json:"md"`
// }

// MX RR. See RFC 1035.
type MX struct {
	Preference uint16 `json:"preference"`
	Mx         string `json:"mx"`
}

// AFSDB RR. See RFC 1183.
type AFSDB struct {
	Subtype  uint16 `json:"subtype"`
	Hostname string `json:"hostname"`
}

// X25 RR. See RFC 1183, Section 3.1.
// type X25 struct {
// 	PSDNAddress string `json:"psdnaddress"`
// }

// RT RR. See RFC 1183, Section 3.3.
//type RT struct {
//	Preference uint16 `json:"preference"`
//	Host       string `json:"host"` // RFC 3597 prohibits compressing records not defined in RFC 1035.
//}

// NS
type NS struct {
	Ns string `json:"ns"`
}

type PTR struct {
	Ptr string `json:"ptr"`
}

// RP RR. See RFC 1138, Section 2.2.
type RP struct {
	Mbox string `json:"mbox"`
	Txt  string `json:"txt"`
}

// TXT RR. See RFC 1035.
type TXT struct {
	Txt []string `json:"txt"`
}

// SPF RR. See RFC 4408, Section 3.1.1.
// type SPF struct {
// 	Txt []string `json:"txt"`
// }

// AVC RR. See https://www.iana.org/assignments/dns-parameters/AVC/avc-completed-template.
// type AVC struct {
// 	Txt []string `json:"txt"`
// }

// SRV RR. See RFC 2782.
type SRV struct {
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
	Port     uint16 `json:"port"`
	Target   string `json:"target"`
}

// NAPTR RR. See RFC 2915.
type NAPTR struct {
	Order       uint16 `json:"order"`
	Preference  uint16 `json:"preference"`
	Flags       string `json:"flags"`
	Service     string `json:"service"`
	Regexp      string `json:"regexp"`
	Replacement string `json:"replacement"`
}

// CERT RR. See RFC 4398.
type CERT struct {
	Type        uint16 `json:"type"`
	KeyTag      uint16 `json:"key"`
	Algorithm   uint8  `json:"algorithm"`
	Certificate string `json:"certificate"`
}

// DNAME RR. See RFC 2672.
type DNAME struct {
	Target string `json:"target"`
}

// A RR. See RFC 1035.
type A struct {
	A net.IP `json:"a"`
}

// AAAA RR. See RFC 3596.
type AAAA struct {
	AAAA net.IP `json:"aaaa"`
}

// PX RR. See RFC 2163.
// type PX struct {
// 	Preference uint16 `json:"preference"`
// 	Map822     string `json:"map822"`
// 	Mapx400    string `json:"mapx400"`
// }

// GPOS RR. See RFC 1712.
// type GPOS struct {
// 	Longitude string `json:"longitude"`
// 	Latitude  string `json:"latitude"`
// 	Altitude  string `json:"altitude"`
// }

// LOC RR. See RFC RFC 1876.
type LOC struct {
	Version   uint8  `json:"version"`
	Size      uint8  `json:"size"`
	HorizPre  uint8  `json:"horiz_pre"`
	VertPre   uint8  `json:"vert_pre"`
	Latitude  uint32 `json:"latitude"`
	Longitude uint32 `json:"longitude"`
	Altitude  uint32 `json:"altitude"`
}

// SIG RR. See RFC 2535. The SIG RR is identical to RRSIG and nowadays only used for SIG(0), See RFC 2931.
// type SIG struct {
// 	RRSIG
// }

// RRSIG RR. See RFC 4034 and RFC 3755.
type RRSIG struct {
	TypeCovered uint16 `json:"type_covered"`
	Algorithm   uint8  `json:"algorithm"`
	Labels      uint8  `json:"labels"`
	OrigTtl     uint32 `json:"orig_ttl"`
	Expiration  uint32 `json:"expiration"`
	Inception   uint32 `json:"inception"`
	KeyTag      uint16 `json:"key_tag"`
	SignerName  string `json:"signer_name"`
	Signature   string `json:"signature"`
}

// NSEC RR. See RFC 4034 and RFC 3755.
type NSEC struct {
	NextDomain string   `json:"next_domain"`
	TypeBitMap []uint16 `json:"type_bit_map"`
}

// TODO(jproxx): fix that
// DLV RR. See RFC 4431.
// type DLV struct{ DS }

// TODO(jproxx): fix that
// CDS RR. See RFC 7344.
// type CDS struct{ DS }

// DS RR. See RFC 4034 and RFC 3658.
type DS struct {
	KeyTag     uint16 `json:"key_tag"`
	Algorithm  uint8  `json:"algorithm"`
	DigestType uint8  `json:"digest_type"`
	Digest     string `json:"digest"`
}

// KX RR. See RFC 2230.
type KX struct {
	Preference uint16 `json:"preference"`
	Exchanger  string `json:"exchanger"`
}

// TA RR. See http://www.watson.org/~weiler/INI1999-19.pdf.
type TA struct {
	KeyTag     uint16 `json:"key_tag"`
	Algorithm  uint8  `json:"algorithm"`
	DigestType uint8  `json:"digest_type"`
	Digest     string `json:"digest"`
}

// type TALINK struct {
// 	PreviousName string `json:"previous_name"`
// 	NextName     string `json:"next_name"`
// }

// SSHFP RR. See RFC RFC 4255.
type SSHFP struct {
	Algorithm   uint8  `json:"algorithm"`
	Type        uint8  `json:"type"`
	FingerPrint string `json:"finger_print"`
}

// KEY RR. See RFC RFC 2535.
//type KEY struct {
//	DNSKEY
//}

// TODO(jproxx): fix that
// CDNSKEY RR. See RFC 7344.
//type CDNSKEY struct {
//	DNSKEY
//}

// DNSKEY RR. See RFC 4034 and RFC 3755.
type DNSKEY struct {
	Flags     uint16 `json:"flags"`
	Protocol  uint8  `json:"protocol"`
	Algorithm uint8  `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

// IPSECKEY RR. See RFC 4025.
type IPSECKEY struct {
	Precedence  uint8  `json:"precedence"`
	GatewayType uint8  `json:"gateway_type"`
	Algorithm   uint8  `json:"algorithm"`
	GatewayAddr net.IP `json:"gateway_addr"` // packing/unpacking/parsing/etc handled together with GatewayHost
	GatewayHost string `json:"gateway_host"`
	PublicKey   string `json:"public_key"`
}

// AMTRELAY RR. See RFC 8777.
// type AMTRELAY struct {
// 	Precedence  uint8  `json:"precedence"`
// 	GatewayType uint8  `json:"gateway_type"` // discovery is packed in here at bit 0x80
// 	GatewayAddr net.IP `json:"gateway_addr"` // packing/unpacking/parsing/etc handled together with GatewayHost
// 	GatewayHost string `json:"gateway_host"`
// }

// RKEY RR. See https://www.iana.org/assignments/dns-parameters/RKEY/rkey-completed-template.
// type RKEY struct {
// 	Flags     uint16 `json:"flags"`
// 	Protocol  uint8  `json:"protocol"`
// 	Algorithm uint8  `json:"algorithm"`
// 	PublicKey string `json:"public_key"`
// }

// NSAPPTR RR. See RFC 1348.
// type NSAPPTR struct {
// 	Ptr string `json:"ptr"`
// }

// NSEC3 RR. See RFC 5155.
type NSEC3 struct {
	Hash       uint8    `json:"hash"`
	Flags      uint8    `json:"flags"`
	Iterations uint16   `json:"iterations"`
	SaltLength uint8    `json:"salt_length"`
	Salt       string   `json:"salt"`
	HashLength uint8    `json:"hash_length"`
	NextDomain string   `json:"next_domain"`
	TypeBitMap []uint16 `json:"type_bit_map"`
}

// NSEC3PARAM RR. See RFC 5155.
type NSEC3PARAM struct {
	Hash       uint8  `json:"hash"`
	Flags      uint8  `json:"flags"`
	Iterations uint16 `json:"iterations"`
	SaltLength uint8  `json:"salt_length"`
	Salt       string `json:"salt"`
}

// TKEY RR. See RFC 2930.
type TKEY struct {
	Algorithm  string `json:"algorithm"`
	Inception  uint32 `json:"inception"`
	Expiration uint32 `json:"expiration"`
	Mode       uint16 `json:"mode"`
	Error      uint16 `json:"error"`
	KeySize    uint16 `json:"key_size"`
	Key        string `json:"key"`
	OtherLen   uint16 `json:"other_len"`
	OtherData  string `json:"other_data"`
}

// RFC3597 represents an unknown/generic RR. See RFC 3597.
// type RFC3597 struct {
// 	Rdata string `json:"rdata"`
// }

// URI RR. See RFC 7553.
type URI struct {
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
	Target   string `json:"target"`
}

// DHCID RR. See RFC 4701.
type DHCID struct {
	Digest string `json:"digest"`
}

// TLSA RR. See RFC 6698.
type TLSA struct {
	Usage        uint8  `json:"usage"`
	Selector     uint8  `json:"selector"`
	MatchingType uint8  `json:"matching_type"`
	Certificate  string `json:"certificate"`
}

// SMIMEA RR. See RFC 8162.
type SMIMEA struct {
	Usage        uint8  `json:"usage"`
	Selector     uint8  `json:"selector"`
	MatchingType uint8  `json:"matching_type"`
	Certificate  string `json:"certificate"`
}

// HIP RR. See RFC 8005.
type HIP struct {
	HitLength          uint8    `json:"hit_length"`
	PublicKeyAlgorithm uint8    `json:"public_key_algorithm"`
	PublicKeyLength    uint16   `json:"public_key_length"`
	Hit                string   `json:"hit"`
	PublicKey          string   `json:"public_key"`
	RendezvousServers  []string `json:"rendezvous_servers"`
}

// NINFO RR. See https://www.iana.org/assignments/dns-parameters/NINFO/ninfo-completed-template.
// type NINFO struct {
// 	ZSData []string `json:"zsdata"`
// }

// NID RR. See RFC RFC 6742.
// type NID struct {
// 	Preference uint16 `json:"preference"`
// 	NodeID     uint64 `json:"node_id"`
// }

// L32 RR, See RFC 6742.
// type L32 struct {
// 	Preference uint16 `json:"preference"`
// 	Locator32  net.IP `json:"locator32"`
// }

// L64 RR, See RFC 6742.
// type L64 struct {
// 	Preference uint16 `json:"preference"`
// 	Locator64  uint64 `json:"locator64"`
// }

// LP RR. See RFC 6742.
// type LP struct {
// 	Preference uint16 `json:"preference"`
// 	Fqdn       string `json:"fqdn"`
// }

// EUI48 RR. See RFC 7043.
type EUI48 struct {
	Address uint64 `json:"address"`
}

// EUI64 RR. See RFC 7043.
type EUI64 struct {
	Address uint64 `json:"address"`
}

// CAA RR. See RFC 6844.
type CAA struct {
	Flag  uint8  `json:"flag"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// EID RR. See http://ana-3.lcs.mit.edu/~jnc/nimrod/dns.txt.
// type EID struct {
// 	Endpoint string `json:"endpoint"`
// }

// type NIMLOC struct {
// 	Locator string `json:"locator"`
// }

// OPENPGPKEY RR. See RFC 7929.
type OPENPGPKEY struct {
	PublicKey string `json:"public_key"`
}

// CSYNC RR. See RFC 7477.
type CSYNC struct {
	Serial     uint32   `json:"serial"`
	Flags      uint16   `json:"flags"`
	TypeBitMap []uint16 `json:"nsec"`
}

// ZONEMD RR, from draft-ietf-dnsop-dns-zone-digest
type ZONEMD struct {
	Serial uint32 `json:"serial"`
	Scheme uint8  `json:"scheme"`
	Hash   uint8  `json:"hash"`
	Digest string `json:"digest"`
}

// TODO(jproxx): fix that
// APL RR. See RFC 3123.
// type APL struct {
// 	Prefixes []APLPrefix `json:"prefixes"`
// }
// TODO(jproxx): fix that
// there is no APLPrefix record!!!
// APLPrefix is an address prefix hold by an APL record.
// type APLPrefix struct {
// 	Negation bool      `json:"negation"`
// 	Network  net.IPNet `json:"network"`
// }
