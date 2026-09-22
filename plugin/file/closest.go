package file

import (
	"github.com/coredns/coredns/plugin/file/tree"

	"github.com/miekg/dns"
)

// ClosestEncloser returns the closest encloser for qname.
func (z *Zone) ClosestEncloser(qname string) (*tree.Elem, bool) {
	offset, end := dns.NextLabel(qname, 0)
	for !end {
		elem, _ := z.Search(qname)
		if elem != nil {
			return elem, true
		}
		qname = qname[offset:]

		offset, end = dns.NextLabel(qname, 0)
	}

	return z.Search(z.origin)
}
