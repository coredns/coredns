package file

import (
	"github.com/miekg/coredns/middleware/file/tree"
	"github.com/miekg/dns"
)

// ClosestEncloser returns the closest encloser for rr. If nil
// is returned you should use the Apex name.
// Move to Tree
func (z *Zone) ClosestEncloser(qname string) *tree.Elem {

	offset, end := dns.NextLabel(qname, 0)
	for !end {
		elem, _ := z.Tree.Search(qname)
		if elem != nil {
			if !elem.Empty() { // only return when not a empty-non-terminal
				return elem
			}
		}
		qname = qname[offset:]

		offset, end = dns.NextLabel(qname, offset)
	}

	return nil // use apex
}

// nameErrorProof finds the closest encloser and return an NSEC that proofs
// the wildcard does (or does) not exist and an NSEC that proofs the name does (or does not) exist.
func (z *Zone) errorProof(qname string, qtype uint16) []dns.RR {

	elem := z.Tree.Prev(qname)
	if elem == nil {
		return nil
	}
	nsec := z.lookupNSEC(elem, true)
	ceName := elem.Name()

	println("Closest encloser", ceName)

	ss := z.ClosestEncloser("*." + ceName)
	ssName := ""
	nsec2 := []dns.RR{}
	if ss == nil {
		ssName = z.origin
		apex, _ := z.Tree.Search(ssName)
		nsec2 = z.lookupNSEC(apex, true)
	} else {
		ssName = ss.Name()
		nsec2 = z.lookupNSEC(ss, true)
	}
	if ceName == ssName {
		return nsec
	}

	return append(nsec, nsec2...)
}
