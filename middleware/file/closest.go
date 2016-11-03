package file

import "github.com/miekg/dns"

// ClosestEncloser returns the closest encloser for rr.
func (z *Zone) ClosestEncloser(qname string) string {

	offset, end := dns.NextLabel(qname, 0)
	for !end {
		elem, _ := z.Tree.Search(qname)
		if elem != nil {
			if !elem.Empty() { // only return when not a empty-non-terminal
				return elem.Name()
			}
		}
		qname = qname[offset:]

		offset, end = dns.NextLabel(qname, offset)
	}

	return z.Apex.SOA.Header().Name
}

// nameErrorProof finds the closest encloser and return an NSEC that proofs
// the wildcard does not exist and an NSEC that proofs the name does no exist.
func (z *Zone) errorProof(qname string, qtype uint16) []dns.RR {

	ce := z.ClosestEncloser(qname)
	println("Closest encloser", ce)

	println("ss", "*."+ce)

	return nil

	/*
		ce := z.ClosestEncloser(qname, qtype)
		println("Closest encloser", ce)
		elem = z.Tree.Prev("*." + ce)
		if elem == nil {
			// Root?
			return nil
		}
		nsec1 := z.lookupNSEC(elem, true)
		nsec1Index := 0
		for i := 0; i < len(nsec1); i++ {
			if nsec1[i].Header().Rrtype == dns.TypeNSEC {
				nsec1Index = i
				break
			}
		}

		if len(nsec) == 0 || len(nsec1) == 0 {
			return nsec
		}

		// Check for duplicate NSEC.
		if nsec[nsecIndex].Header().Name == nsec1[nsec1Index].Header().Name &&
			nsec[nsecIndex].(*dns.NSEC).NextDomain == nsec1[nsec1Index].(*dns.NSEC).NextDomain {
			return nsec
		}

		return append(nsec, nsec1...)
	*/
}
