// Package rrutil provides function to find certain RRs in slices.
package rrutil

import "github.com/miekg/dns"

// SubTypeSignature returns the RRSIG for the subtype.
func SubTypeSignature(rrs []dns.RR, subtype uint16) []dns.RR {
	sigs := []dns.RR{}
	// there may be multiple keys that have signed this subtype
	for _, sig := range rrs {
		if s, ok := sig.(*dns.RRSIG); ok {
			if s.TypeCovered == subtype {
				sigs = append(sigs, s)
			}
		}
	}
	return sigs
}
