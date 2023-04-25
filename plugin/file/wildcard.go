package file

import "github.com/miekg/dns"

// replaceWithAsteriskLabel replaces the left most label with '*'.
func replaceWithAsteriskLabel(qname string) (wildcard string) {
	i, shot := dns.NextLabel(qname, 0)
	if shot {
		return ""
	}

	return "*." + qname[i:]
}
