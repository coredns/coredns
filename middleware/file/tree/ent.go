package tree

import "github.com/miekg/dns"

// isNonTerminal returns the empty non terminal name and true if qname contains
// an empty non terminal. This means the qname is more than one (or more)
// labels longer then the zone's origin.
func (t *Tree) isNonTerminal(rr dns.RR) ([]string, bool) {
	qname := rr.Header().Name
	qnameLen := dns.CountLabel(qname)

	// a.b.example.org -> b.example.org
	// a.b.c.example.org -> b.example.org and c.example.org
	if qnameLen > t.OrigLen+1 {
		// Skip first label from the right.
		i, _ := dns.NextLabel(qname, 0)
		ent := []string{}
		for j := 0; j < qnameLen-t.OrigLen-1; j++ {
			ent = append(ent, qname[i:])
			i, _ = dns.NextLabel(qname, i)
		}
		return ent, true
	}

	return nil, false
}
