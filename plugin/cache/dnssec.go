package cache

import "github.com/miekg/dns"

// requestDO reads acquisition capability without consulting the transport.
// Unlike request.Request.Do, it also works for clientless prefetch requests.
func requestDO(m *dns.Msg) bool {
	opt := m.IsEdns0()
	return opt != nil && opt.Do()
}

// filterDNSSEC removes authentication records for a DO=0 client. In the
// answer section, an explicitly requested type (including ANY) is retained
// (RFC 4035 section 3.2.1). Pass qtype=0 for authority/additional sections.
// Never compact the input slice: it may be shared with a cached item.
func filterDNSSEC(rrs []dns.RR, qtype uint16) []dns.RR {
	var filtered []dns.RR
	for n, rr := range rrs {
		typ := rr.Header().Rrtype
		remove := false
		switch typ {
		case dns.TypeRRSIG, dns.TypeNSEC, dns.TypeNSEC3, dns.TypeDS, dns.TypeDNSKEY:
			remove = qtype != dns.TypeANY && qtype != typ
		}
		if remove && filtered == nil {
			filtered = make([]dns.RR, 0, len(rrs))
			filtered = append(filtered, rrs[:n]...)
		}
		if !remove && filtered != nil {
			filtered = append(filtered, rr)
		}
	}
	if filtered != nil {
		return filtered
	}
	return rrs
}

// filterRRSlice filters out OPT RRs, and sets all RR TTLs to ttl.
// If dup is true the RRs in rrs are _copied_ before adjusting their
// TTL and the slice of copied RRs is returned.
func filterRRSlice(rrs []dns.RR, ttl uint32, dup bool) []dns.RR {
	n := 0
	for _, r := range rrs {
		if r.Header().Rrtype != dns.TypeOPT {
			n++
		}
	}
	rs := make([]dns.RR, n)
	j := 0
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
	return rs
}
