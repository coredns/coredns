package dnsserver

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"hash"

	"github.com/miekg/dns"
)

type tsigProvider struct {
	TsigSecret    map[string]string
	TsigAlgorithm map[string]string
}

var _ dns.TsigProvider = &tsigProvider{}

// Generate a digest using specific algorithm.
func (p *tsigProvider) Generate(msg []byte, t *dns.TSIG) ([]byte, error) {
	secret, ok := p.match(t)
	if !ok {
		return nil, dns.ErrKey
	}

	// copied from github.com/miekg/dns/tsig.go
	rawsecret, err := fromBase64([]byte(secret))
	if err != nil {
		return nil, err
	}
	var h hash.Hash
	switch dns.CanonicalName(t.Algorithm) {
	case dns.HmacSHA1:
		h = hmac.New(sha1.New, rawsecret)
	case dns.HmacSHA224:
		h = hmac.New(sha256.New224, rawsecret)
	case dns.HmacSHA256:
		h = hmac.New(sha256.New, rawsecret)
	case dns.HmacSHA384:
		h = hmac.New(sha512.New384, rawsecret)
	case dns.HmacSHA512:
		h = hmac.New(sha512.New, rawsecret)
	default:
		return nil, dns.ErrKeyAlg
	}
	h.Write(msg)
	return h.Sum(nil), nil
}

func fromBase64(s []byte) (buf []byte, err error) {
	buflen := base64.StdEncoding.DecodedLen(len(s))
	buf = make([]byte, buflen)
	n, err := base64.StdEncoding.Decode(buf, s)
	buf = buf[:n]
	return
}

// Verify the digest of TSIG RR.
func (p *tsigProvider) Verify(msg []byte, t *dns.TSIG) error {
	b, err := p.Generate(msg, t)
	if err != nil {
		return err
	}
	mac, err := hex.DecodeString(t.MAC)
	if err != nil {
		return err
	}
	if !hmac.Equal(b, mac) {
		return dns.ErrSig
	}
	return nil
}

// match the name and algorithm of TSIG RR. if true, return the TSIG key.
func (p *tsigProvider) match(t *dns.TSIG) (string, bool) {
	// https://datatracker.ietf.org/doc/html/rfc8945#section-1.2-2
	// Use of TSIG presumes prior agreement between the two parties involved
	// (e.g., resolver and server) as to any algorithm and key to be used.
	// The way that this agreement is reached is outside the scope of the document.
	secret, ok := p.TsigSecret[t.Hdr.Name]
	if !ok {
		return "", false
	}

	if p.TsigAlgorithm[t.Hdr.Name] != t.Algorithm {
		return "", false
	}

	return secret, true
}

// NewTsigProvider create a TSIG provider based on exact RR name matching and algorithms for verifying or generating digests.
func NewTsigProvider(tsigSecret, tsigAlgorithm map[string]string) dns.TsigProvider {
	if len(tsigSecret) == 0 || len(tsigAlgorithm) == 0 {
		return nil
	}
	return &tsigProvider{
		TsigSecret:    tsigSecret,
		TsigAlgorithm: tsigAlgorithm,
	}
}
