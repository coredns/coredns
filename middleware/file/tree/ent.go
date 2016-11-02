package tree

import "github.com/miekg/dns"

// isNonTerminal returns the empty non terminal name and true if qname contains
// an empty non terminal. This means the qname is more than one (or more)
// labels longer then the zone's origin.
func (t *Tree) isNonTerminal(rr dns.RR) ([]dns.RR, bool) {
	qname := rr.Header().Name
	qnameLen := dns.CountLabel(qname)

	// a.b.example.org -> b.example.org
	// a.b.c.example.org -> b.example.org and c.example.org
	if qnameLen > t.OrigLen+1 {
		// Skip first label from the right.
		i, _ := dns.NextLabel(qname, 0)
		ent := []dns.RR{}
		for j := 0; j < qnameLen-t.OrigLen-1; j++ {
			// Fake RR we will be using for this.
			a := &dns.ANY{Hdr: dns.RR_Header{Name: qname[i:], Rrtype: dns.TypeANY, Class: dns.ClassANY}}
			ent = append(ent, a)

			i, _ = dns.NextLabel(qname, i)
		}
		return ent, true
	}

	return nil, false
}
