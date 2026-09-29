package file

import (
	"github.com/coredns/coredns/plugin/file/tree"

	"github.com/miekg/dns"
)

// ClosestEncloser returns the closest encloser for qname.
func (z *Zone) ClosestEncloser(qname string) (*tree.Elem, bool) {
	_, zoneTree := z.snapshot()
	if zoneTree == nil {
		return nil, false
	}

	offset, end := dns.NextLabel(qname, 0)
	for !end {
		elem, _ := zoneTree.Search(qname)
		if elem != nil {
			return elem, true
		}
		qname = qname[offset:]

		offset, end = dns.NextLabel(qname, 0)
	}

	return zoneTree.Search(z.origin)
}
