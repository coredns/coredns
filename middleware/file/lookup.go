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

			// DS record

			// If qtype == NS, we should returns success to put RRs in answer.
			return nil, nsrrs, glue, Delegation
		}

		i++
	}

	// Found entire name.
	if found && shot {

		println("HIERE", qname, parts)
		println(elem.Name())
		for _, rr := range elem.All() {
			println(rr.String())
		}

		rrs := elem.Types(qtype, qname)
		if len(rrs) == 0 {
			println("NODATA")
			return z.noData(elem, do)
		}

		if do {
			sigs := elem.Types(dns.TypeRRSIG, qname)
			sigs = signatureForSubType(sigs, qtype)
			rrs = append(rrs, sigs...)
		}

		return rrs, nil, nil, Success

	}

	// previous one should be found, use parts
	// switch on found and do success or nxdomain

	if elem == nil {
		return z.emptyNonTerminal(qname, do)
		return z.nameError(qname, qtype, do)
	}

	rrs := elem.Types(dns.TypeCNAME, qname)
	if len(rrs) > 0 { // should only ever be 1 actually; TODO(miek) check for this?
		return z.lookupCNAME(rrs, qtype, do)
	}

	rrs = elem.Types(qtype, qname)
	if len(rrs) == 0 {
		return z.noData(elem, do)
	}

	if do {
		sigs := elem.Types(dns.TypeRRSIG, qname)
		sigs = signatureForSubType(sigs, qtype)
		rrs = append(rrs, sigs...)
	}

	return rrs, nil, nil, Success
}

func (z *Zone) noData(elem *tree.Elem, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	soa, _, _, _ := z.lookupSOA(do)
	nsec := z.lookupNSEC(elem, do)
	return nil, append(soa, nsec...), nil, Success
}

func (z *Zone) emptyNonTerminal(qname string, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	soa, _, _, _ := z.lookupSOA(do)

	elem := z.Tree.Prev(qname)
	nsec := z.lookupNSEC(elem, do)
	return nil, append(soa, nsec...), nil, Success
}

func (z *Zone) nameError(qname string, qtype uint16, do bool) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	// Is there a wildcard?
	ce := z.ClosestEncloser(qname, qtype)
	elem, _ := z.Tree.Search("*."+ce, qtype) // use result here?

	if elem != nil {
		ret := elem.Types(qtype) // there can only be one of these (or zero)
		switch {
		case ret != nil:
			if do {
				sigs := elem.Types(dns.TypeRRSIG)
				sigs = signatureForSubType(sigs, qtype)
				ret = append(ret, sigs...)
			}
			ret = wildcardReplace(qname, ce, ret)
			return ret, nil, nil, Success
		case ret == nil:
			// nodata, nsec from the wildcard - type does not exist
			// nsec proof that name does not exist
			// TODO(miek)
		}
	}

	// name error
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
	nsec := elem.Types(dns.TypeNSEC)
	if do {
		sigs := elem.Types(dns.TypeRRSIG)
		sigs = signatureForSubType(sigs, dns.TypeNSEC)
		if len(sigs) > 0 {
			nsec = append(nsec, sigs...)
		}
	}
	return nsec
}

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

// wildcardReplace replaces the ownername with the original query name.
func wildcardReplace(qname, ce string, rrs []dns.RR) []dns.RR {
	// need to copy here, otherwise we change in zone stuff
	ret := make([]dns.RR, len(rrs))
	for i, r := range rrs {
		ret[i] = dns.Copy(r)
		ret[i].Header().Name = qname
	}
	return ret
}
