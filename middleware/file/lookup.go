package file

import (
	"github.com/miekg/coredns/middleware/file/tree"

	"github.com/miekg/dns"
)

// Result is the result of a Lookup
type Result int

const (
	// Success is a successful lookup.
	Success Result = iota
	// NameError indicates a nameerror
	NameError
	// Delegation indicates the lookup resulted in a delegation.
	Delegation
	// NoData indicates the lookup resulted in a NODATA.
	NoData
	// ServerFailure indicates a server failure during the lookup.
	ServerFailure
)

// Lookup looks up qname and qtype in the zone. When do is true DNSSEC records are included.
// Three sets of records are returned, one for the answer, one for authority  and one for the additional section.
func (z *Zone) Lookup(qname string, qtype uint16, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	if !z.NoReload {
		z.reloadMu.RLock()
	}
	defer func() {
		if !z.NoReload {
			z.reloadMu.RUnlock()
		}
	}()

	if qtype == dns.TypeSOA {
		r1, r2, r3, res := z.lookupSOA(do)
		return r1, r2, r3, res
	}
	if qtype == dns.TypeNS && qname == z.origin {
		r1, r2, r3, res := z.lookupNS(do)
		return r1, r2, r3, res
	}

	var (
		found, shot bool
		parts       string
		i           int
		elem        *tree.Elem
	)

	// Lookups
	// * per label from the right, start with the zone name
	// when found; check NS records
	//
	// first not found, try wildcard, *.+name
	// check NS records + DS
	// complete found, chceck types

	// * hit for either check types, special case cname and dname and friends
	// * for delegation i.e. NS records, get DS

	for {
		parts, shot = z.nameFromRight(qname, i)
		// We overshot the name, break and check if we previously found something.
		if shot {
			break
		}

		elem, found = z.Tree.Search(parts, qtype)
		if !found {
			break
		}

		// If we see NS records, it means the name as been delegated.
		if nsrrs := elem.Types(dns.TypeNS); nsrrs != nil {

			glue := []dns.RR{}
			for _, ns := range nsrrs {
				if dns.IsSubDomain(ns.Header().Name, ns.(*dns.NS).Ns) {
					// TODO(miek): lowercase... Or make case compare not care, in bytes.Compare?
					glue = append(glue, z.lookupGlue(ns.(*dns.NS).Ns)...)
				}
			}

			// If qtype == NS, we should returns success to put RRs in answer.
			if qtype == dns.TypeNS {
				return nsrrs, nil, glue, Success
			}

			dss := z.lookupDS(elem.Name())
			nsrrs = append(nsrrs, dss...)

			return nil, nsrrs, glue, Delegation
		}

		i++
	}

	// What does found and !shot mean - do we ever hit it
	if found && !shot {
		println("found && !shot", qname, qtype, z.origin)
	}

	// Found entire name.
	if found && shot {

		if rrs := elem.Types(dns.TypeCNAME, qname); len(rrs) > 0 {
			return z.lookupCNAME(rrs, qtype, do)
		}

		rrs := elem.Types(qtype, qname)
		if len(rrs) == 0 {
			// closest encloser

			return z.noData(elem, do)
		}

		if do {
			sigs := elem.Types(dns.TypeRRSIG, qname)
			sigs = signatureForSubType(sigs, qtype)
			rrs = append(rrs, sigs...)
		}

		return rrs, nil, nil, Success

	}

	// Not found
	// Do we have a wildcard for the name parts that we do have?
	wildcard := "*." + parts
	if elem, found := z.Tree.Search(wildcard, qtype); found {
		println("wildcard found", wildcard)
		// wildcard foo here
		elem = elem
	}

	// No wildcard

	return z.nameError(qname, qtype, do)

	return nil, nil, nil, ServerFailure
}

func (z *Zone) noData(elem *tree.Elem, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	println("NODATA RESPONSE")

	soa, _, _, _ := z.lookupSOA(do)
	nsec := z.lookupNSEC(elem, do)
	return nil, append(soa, nsec...), nil, Success
}

func (z *Zone) nameError(qname string, qtype uint16, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	ret := []dns.RR{z.Apex.SOA}
	if do {
		ret = append(ret, z.Apex.SIGSOA...)
		ret = append(ret, z.nameErrorProof(qname, qtype)...)
	}
	return nil, ret, nil, NameError
}

// lookupGlue looks up A and AAAA for name.
func (z *Zone) lookupGlue(name string) []dns.RR {
	glue := []dns.RR{}

	// A
	if elem, found := z.Tree.Search(name, dns.TypeA); found {
		glue = append(glue, elem.Types(dns.TypeA)...)
	}

	// AAAA
	if elem, found := z.Tree.Search(name, dns.TypeAAAA); found {
		glue = append(glue, elem.Types(dns.TypeAAAA)...)
	}
	return glue
}

// lookupDS looks up DS and sigs
func (z *Zone) lookupDS(name string) []dns.RR {
	dss := []dns.RR{}

	if elem, found := z.Tree.Search(name, dns.TypeDS); found {
		dss = elem.Types(dns.TypeDS)
		sigs := elem.Types(dns.TypeRRSIG)
		sigs = signatureForSubType(sigs, dns.TypeDS)
		if len(sigs) > 0 {
			dss = append(dss, sigs...)
		}
	}

	return dss
}

func (z *Zone) lookupSOA(do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	if do {
		ret := append([]dns.RR{z.Apex.SOA}, z.Apex.SIGSOA...)
		return ret, nil, nil, Success
	}
	return []dns.RR{z.Apex.SOA}, nil, nil, Success
}

func (z *Zone) lookupNS(do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	if do {
		ret := append(z.Apex.NS, z.Apex.SIGNS...)
		return ret, nil, nil, Success
	}
	return z.Apex.NS, nil, nil, Success
}

// lookupNSEC looks up nsec and sigs.
func (z *Zone) lookupNSEC(elem *tree.Elem, do bool) []dns.RR {
	if !do {
		return nil
	}
	// Need closest en
	nsec := elem.Types(dns.TypeNSEC)
	sigs := elem.Types(dns.TypeRRSIG)
	sigs = signatureForSubType(sigs, dns.TypeNSEC)
	if len(sigs) > 0 {
		nsec = append(nsec, sigs...)
	}
	return nsec
}

// TODO(miek): Needs to be recursive and do multiple lookups for CNAME chains.
func (z *Zone) lookupCNAME(rrs []dns.RR, qtype uint16, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	elem, _ := z.Tree.Search(rrs[0].(*dns.CNAME).Target, qtype)
	if elem == nil {
		return rrs, nil, nil, Success
	}

	targets := cnameForType(elem.All(), qtype)
	if do {
		sigs := elem.Types(dns.TypeRRSIG)
		sigs = signatureForSubType(sigs, qtype)
		if len(sigs) > 0 {
			targets = append(targets, sigs...)
		}
	}
	return append(rrs, targets...), nil, nil, Success
}

func cnameForType(targets []dns.RR, origQtype uint16) []dns.RR {
	ret := []dns.RR{}
	for _, target := range targets {
		if target.Header().Rrtype == origQtype {
			ret = append(ret, target)
		}
	}
	return ret
}

// signatureForSubType range through the signature and return the correct ones for the subtype.
func signatureForSubType(rrs []dns.RR, subtype uint16) []dns.RR {
	sigs := []dns.RR{}
	for _, sig := range rrs {
		if s, ok := sig.(*dns.RRSIG); ok {
			if s.TypeCovered == subtype {
				sigs = append(sigs, s)
			}
		}
	}
	return sigs
}
