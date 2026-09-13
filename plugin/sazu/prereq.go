package sazu

import (
	"fmt"

	"github.com/miekg/dns"
)

// EvaluatePrerequisites checks RFC 2136 §2.4's five prerequisite forms
// against zone's current content, in order, failing on the first one not
// satisfied. Distinguishing the five forms relies on Class and Rdlength
// exactly as they appear on the wire -- correct for prereqs as unpacked
// from a received message (which is the only place this is meant to be
// called from), not for freshly-constructed-in-Go RRs, whose zero-value
// Rdlength would be indistinguishable from a real value-independent
// prerequisite.
//
// Returns the RFC 2136-appropriate rcode for the first failure (matching
// §2.6's rcode table) and a human-readable reason, or (dns.RcodeSuccess,
// nil) if every prerequisite holds.
func EvaluatePrerequisites(zone *ZoneData, prereqs []dns.RR, zclass uint16) (int, error) {
	for _, rr := range prereqs {
		h := rr.Header()
		switch {
		case h.Class == dns.ClassANY && h.Rrtype == dns.TypeANY && h.Rdlength == 0:
			// §2.4.4 Name is in use.
			if !zone.NameExists(h.Name) {
				return dns.RcodeNameError, fmt.Errorf("name %s is not in use", h.Name)
			}
		case h.Class == dns.ClassNONE && h.Rrtype == dns.TypeANY && h.Rdlength == 0:
			// §2.4.5 Name is not in use.
			if zone.NameExists(h.Name) {
				return dns.RcodeYXDomain, fmt.Errorf("name %s is already in use", h.Name)
			}
		case h.Class == dns.ClassANY && h.Rdlength == 0:
			// §2.4.1 RRset exists (value-independent).
			if len(zone.Lookup(h.Name, h.Rrtype)) == 0 {
				return dns.RcodeNXRrset, fmt.Errorf("rrset %s/%s does not exist", h.Name, dns.TypeToString[h.Rrtype])
			}
		case h.Class == dns.ClassNONE && h.Rdlength == 0:
			// §2.4.3 RRset does not exist.
			if len(zone.Lookup(h.Name, h.Rrtype)) != 0 {
				return dns.RcodeYXRrset, fmt.Errorf("rrset %s/%s exists but must not", h.Name, dns.TypeToString[h.Rrtype])
			}
		case h.Class == zclass:
			// §2.4.2 RRset exists (value-dependent) -- the SOA-serial
			// staleness guard uses exactly this form.
			existing := zone.Lookup(h.Name, h.Rrtype)
			matched := false
			for _, e := range existing {
				if rrEqualContent(e, rr) {
					matched = true
					break
				}
			}
			if !matched {
				return dns.RcodeNXRrset, fmt.Errorf(
					"rrset %s/%s does not currently match the required value (stale push?)", h.Name, dns.TypeToString[h.Rrtype])
			}
		default:
			return dns.RcodeFormatError, fmt.Errorf("malformed prerequisite for %s", h.Name)
		}
	}
	return dns.RcodeSuccess, nil
}

// ApplyUpdateOps applies RFC 2136 §2.5's four update forms to zone, in
// order. Same reliance on wire-accurate Class/Rdlength as
// EvaluatePrerequisites. Callers are expected to have already evaluated
// prerequisites and authenticated the request -- this function performs
// no checks of its own beyond recognizing which of the four forms each
// op is.
func ApplyUpdateOps(zone *ZoneData, ops []dns.RR, zclass uint16) error {
	for _, rr := range ops {
		h := rr.Header()
		switch {
		case h.Class == zclass:
			// §2.5.1 Add to an RRset.
			zone.Insert(rr)
		case h.Class == dns.ClassANY && h.Rrtype == dns.TypeANY && h.Rdlength == 0:
			// §2.5.3 Delete all RRsets from a name.
			zone.DeleteName(h.Name)
		case h.Class == dns.ClassANY && h.Rdlength == 0:
			// §2.5.2 Delete an RRset.
			zone.DeleteRRset(h.Name, h.Rrtype)
		case h.Class == dns.ClassNONE:
			// §2.5.4 Delete an RR from an RRset.
			zone.DeleteRR(rr)
		default:
			return fmt.Errorf("malformed update op for %s", h.Name)
		}
	}
	return nil
}
