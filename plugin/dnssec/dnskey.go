package dnssec

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"os"
	"time"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"

	"golang.org/x/crypto/ed25519"
)

// DNSKEY holds a DNSSEC public and private key used for on-the-fly signing.
type DNSKEY struct {
	K   *dns.DNSKEY
	D   *dns.DS
	s   crypto.Signer
	tag uint16
}

// ParseKeyFile read a DNSSEC keyfile as generated by dnssec-keygen or other
// utilities. It adds ".key" for the public key and ".private" for the private key.
func ParseKeyFile(pubFile, privFile string) (*DNSKEY, error) {
	f, e := os.Open(pubFile)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	k, e := dns.ReadRR(f, pubFile)
	if e != nil {
		return nil, e
	}

	f, e = os.Open(privFile)
	if e != nil {
		return nil, e
	}
	defer f.Close()

	dk, ok := k.(*dns.DNSKEY)
	if !ok {
		return nil, errors.New("no public key found")
	}
	p, e := dk.ReadPrivateKey(f, privFile)
	if e != nil {
		return nil, e
	}

	if s, ok := p.(*rsa.PrivateKey); ok {
		return &DNSKEY{K: dk, D: dk.ToDS(dns.SHA256), s: s, tag: dk.KeyTag()}, nil
	}
	if s, ok := p.(*ecdsa.PrivateKey); ok {
		return &DNSKEY{K: dk, D: dk.ToDS(dns.SHA256), s: s, tag: dk.KeyTag()}, nil
	}
	if s, ok := p.(ed25519.PrivateKey); ok {
		return &DNSKEY{K: dk, D: dk.ToDS(dns.SHA256), s: s, tag: dk.KeyTag()}, nil
	}
	return &DNSKEY{K: dk, D: dk.ToDS(dns.SHA256), s: nil, tag: 0}, errors.New("no private key found")
}

// getDNSKEY returns the correct DNSKEY to the client. Signatures are added when do is true.
func (d Dnssec) getDNSKEY(state request.Request, zone string, do bool, server string) *dns.Msg {
	keys := make([]dns.RR, len(d.keys))
	for i, k := range d.keys {
		keys[i] = dns.Copy(k.K)
		keys[i].Header().Name = zone
	}
	m := new(dns.Msg)
	m.SetReply(state.Req)
	m.Answer = keys
	if !do {
		return m
	}

	incep, expir := incepExpir(time.Now().UTC())
	if sigs, err := d.sign(keys, zone, 3600, incep, expir, server); err == nil {
		m.Answer = append(m.Answer, sigs...)
	}
	return m
}

// Return true if this is a zone key with the SEP bit unset. This implies a ZSK (rfc4034 2.1.1).
func (k DNSKEY) isZSK() bool {
	return k.K.Flags&(1<<8) == (1<<8) && k.K.Flags&1 == 0
}

// Return true if this is a zone key with the SEP bit set. This implies a KSK (rfc4034 2.1.1).
func (k DNSKEY) isKSK() bool {
	return k.K.Flags&(1<<8) == (1<<8) && k.K.Flags&1 == 1
}
