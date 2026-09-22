package loadbalance

import (
	"net"

	"github.com/miekg/dns"
)

func reorderPreferredSubnets(msg *dns.Msg, subnets []*net.IPNet) *dns.Msg {
	msg.Answer = reorderRecords(msg.Answer, subnets)
	msg.Extra = reorderRecords(msg.Extra, subnets)
	return msg
}

func reorderRecords(records []dns.RR, subnets []*net.IPNet) []dns.RR {
	var cname, address, mx, rest []dns.RR

	for _, r := range records {
		switch r.Header().Rrtype {
		case dns.TypeCNAME:
			cname = append(cname, r)
		case dns.TypeA, dns.TypeAAAA:
			address = append(address, r)
		case dns.TypeMX:
			mx = append(mx, r)
		default:
			rest = append(rest, r)
		}
	}

	sorted := sortBySubnetPriority(address, subnets)

	out := append([]dns.RR{}, cname...)
	out = append(out, sorted...)
	out = append(out, mx...)
	out = append(out, rest...)
	return out
}

func sortBySubnetPriority(records []dns.RR, subnets []*net.IPNet) []dns.RR {
	matched := make([]dns.RR, 0, len(records))
	seen := make(map[int]bool)

	for _, subnet := range subnets {
		for i, r := range records {
			if seen[i] {
				continue
			}
			ip := extractIP(r)
			if ip != nil && subnet.Contains(ip) {
				matched = append(matched, r)
				seen[i] = true
			}
		}
	}

	unmatched := make([]dns.RR, 0, len(records)-len(matched))
	for i, r := range records {
		if !seen[i] {
			unmatched = append(unmatched, r)
		}
	}

	return append(matched, unmatched...)
}

func extractIP(rr dns.RR) net.IP {
	switch r := rr.(type) {
	case *dns.A:
		return r.A
	case *dns.AAAA:
		return r.AAAA
	default:
		return nil
	}
}
