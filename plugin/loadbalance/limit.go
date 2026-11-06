package loadbalance

import "github.com/miekg/dns"

// limitAnswers selects the ordered prefix of each address or MX answer RRset.
// Alias chains and other record types do not consume the limit. This changes
// the answer data, so it must not be applied to authenticated or signed data.
func limitAnswers(res *dns.Msg, limit int) {
	if len(res.Answer) <= limit || res.AuthenticatedData || res.Truncated {
		return
	}
	for _, section := range [][]dns.RR{res.Answer, res.Ns, res.Extra} {
		for _, rr := range section {
			switch rr.Header().Rrtype {
			case dns.TypeRRSIG, dns.TypeSIG, dns.TypeTSIG:
				return
			}
		}
	}
	type rrset struct {
		name          string
		rrtype, class uint16
	}
	counts := make(map[rrset]int)
	var name [256]byte
	var answer []dns.RR
	for i, rr := range res.Answer {
		h := rr.Header()
		switch h.Rrtype {
		case dns.TypeA, dns.TypeAAAA, dns.TypeMX:
			// Compare DNS names as octets, including equivalent escaped forms.
			// Label lengths are at most 63, so ASCII folding cannot change them.
			n, err := dns.PackDomainName(h.Name, name[:], 0, nil, false)
			if err != nil {
				return
			}
			for j, b := range name[:n] {
				if b >= 'A' && b <= 'Z' {
					name[j] += 'a' - 'A'
				}
			}
			key := rrset{string(name[:n]), h.Rrtype, h.Class}
			counts[key]++
			if counts[key] > limit {
				if answer == nil {
					// A backend can own the original slice; do not filter it in place.
					answer = make([]dns.RR, i, len(res.Answer)-1)
					copy(answer, res.Answer[:i])
				}
				continue
			}
		}
		if answer != nil {
			answer = append(answer, rr)
		}
	}
	if answer != nil {
		res.Answer = answer
	}
}
