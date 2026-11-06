package autopath

import (
	"strings"

	"github.com/miekg/dns"
)

// cnamer will prefix the answer section with a cname that points from original qname to the
// name of the first RR. It will also update the question section and put original in there.
func cnamer(m *dns.Msg, original string) {
	for _, a := range m.Answer {
		if strings.EqualFold(original, a.Header().Name) {
			continue
		}
		// Build a new slice: m.Answer may be owned by the next plugin (plugin/file answers
		// straight out of the zone tree), so it must not be modified in place.
		answer := make([]dns.RR, len(m.Answer)+1)
		answer[0] = &dns.CNAME{
			Hdr:    dns.RR_Header{Name: original, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: a.Header().Ttl},
			Target: a.Header().Name,
		}
		copy(answer[1:], m.Answer)
		m.Answer = answer
		break
	}
	m.Question[0].Name = original
}
