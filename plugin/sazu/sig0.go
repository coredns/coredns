package sazu

import (
	"crypto/ed25519"
	"time"

	"github.com/miekg/dns"
)

// SignUpdate signs m (an already-built RFC 2136 UPDATE message, question
// section set, no SIG(0) yet) with priv, acting as signerKey, and returns
// the exact wire bytes to send: SIG.Sign packs the message, appends the
// SIG(0) record to the additional section, and fixes up the header count
// itself, per RFC 2931 §3.1 -- there is no separate re-encode step here,
// which matters because RFC 2931 signs literal wire bytes, not a
// re-serialization of parsed fields (the exact issue the earlier Rust/rDNS
// port had to handle by hand; miekg/dns's SIG.Sign already does this
// correctly).
func SignUpdate(m *dns.Msg, signerKey *dns.DNSKEY, priv ed25519.PrivateKey, inception, expiration time.Time) ([]byte, error) {
	sig := &dns.SIG{
		RRSIG: dns.RRSIG{
			Hdr:        dns.RR_Header{Name: ".", Rrtype: dns.TypeSIG, Class: dns.ClassANY, Ttl: 0},
			Algorithm:  signerKey.Algorithm,
			KeyTag:     signerKey.KeyTag(),
			SignerName: signerKey.Hdr.Name,
			Inception:  uint32(inception.Unix()),
			Expiration: uint32(expiration.Unix()),
		},
	}
	return sig.Sign(priv, m)
}

// VerifySIG0 verifies a SIG(0)-signed message's raw wire bytes (as
// received, not re-packed -- see SignUpdate's doc comment for why that
// distinction matters) against candidateKey. It also checks the
// signature's validity window, which SIG.Verify itself already enforces
// internally (unlike RRSIG.Verify, whose caller must check it separately).
func VerifySIG0(raw []byte, candidateKey *dns.DNSKEY) error {
	m := new(dns.Msg)
	if err := m.Unpack(raw); err != nil {
		return err
	}
	sigRR := isSig0(m)
	if sigRR == nil {
		return chainErr("sig0", "message has no trailing SIG(0) record")
	}
	key := &dns.KEY{DNSKEY: *candidateKey}
	return sigRR.Verify(key, raw)
}

// isSig0 checks whether m's last additional-section record is a SIG(0)
// transaction signature (a SIG RR with TypeCovered 0, RFC 2931 §3), the
// same way Msg.IsTsig checks for a trailing TSIG.
func isSig0(m *dns.Msg) *dns.SIG {
	if len(m.Extra) == 0 {
		return nil
	}
	last := m.Extra[len(m.Extra)-1]
	sig, ok := last.(*dns.SIG)
	if !ok || sig.Header().Rrtype != dns.TypeSIG || sig.TypeCovered != 0 {
		return nil
	}
	return sig
}
