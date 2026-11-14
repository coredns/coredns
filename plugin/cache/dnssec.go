package cache

import "github.com/miekg/dns"

// requestDO reads acquisition capability without consulting the transport.
// Unlike request.Request.Do, it also works for clientless prefetch requests.
func requestDO(m *dns.Msg) bool {
	opt := m.IsEdns0()
	return opt != nil && opt.Do()
}

// stripDNSSEC adapts m, a response acquired with DO, for a client without DO.
// It removes the authenticating DNSSEC records but not those of the
// explicitly requested type, in any section (RFC 4035 section 3.2.1), and
// keeps the whole answer to ANY. A response acquired without DO is already
// what upstream returns to such a client and is served unchanged.
func stripDNSSEC(m *dns.Msg, qtype uint16) {
	m.Answer = filterDNSSEC(m.Answer, qtype)
	if qtype == dns.TypeANY {
		// ANY asks for every RRset at QNAME, not for the authenticating
		// records in the other sections.
		qtype = 0
	}
	m.Ns = filterDNSSEC(m.Ns, qtype)
	m.Extra = filterDNSSEC(m.Extra, qtype)
}

// filterDNSSEC removes authenticating DNSSEC records other than those of type
// qtype: ANY keeps all of them and 0 none.
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
