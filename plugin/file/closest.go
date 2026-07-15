package file

import (
	"github.com/coredns/coredns/plugin/file/tree"

	"github.com/miekg/dns"
)

// ClosestEncloser returns the closest encloser for qname. It searches tr, which
// the caller pins from a single zone snapshot so the lookup stays consistent
// even if a reload or transfer swaps the zone's tree concurrently.
func (z *Zone) ClosestEncloser(tr *tree.Tree, qname string) (*tree.Elem, bool) {
	offset, end := dns.NextLabel(qname, 0)
	for !end {
		elem, _ := tr.Search(qname)
		if elem != nil {
			return elem, true
		}
		qname = qname[offset:]

		offset, end = dns.NextLabel(qname, 0)
	}

	return tr.Search(z.origin)
}
