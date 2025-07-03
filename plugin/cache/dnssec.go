package cache

import "github.com/miekg/dns"

// filterRRSlice filters out OPT RRs, and sets all RR TTLs to ttl.
// If dup is true the RRs in rrs are _copied_ before adjusting their
// TTL and the slice of copied RRs is returned.
func filterRRSlice(rrs []dns.RR, ttl uint32, dup bool) []dns.RR {
	j := 0
	rs := make([]dns.RR, len(rrs))
	for _, r := range rrs {
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}
		if dup {
			rs[j] = dns.Copy(r)
		} else {
			rs[j] = r
		}
		rs[j].Header().Ttl = ttl
		j++
	}
	return rs[:j]
}
