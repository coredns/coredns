//go:build coredns_all || coredns_file || coredns_auto || coredns_secondary || coredns_route53 || coredns_azure || coredns_clouddns || coredns_sign

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
