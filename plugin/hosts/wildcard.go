package hosts

import "github.com/miekg/dns"

// isWildcardName reports whether name is a wildcard owner name (*.example.com.).
func isWildcardName(name string) bool {
	return len(name) >= 2 && name[0] == '*' && name[1] == '.'
}

// replaceWithAsteriskLabel replaces the leftmost label with '*'.
func replaceWithAsteriskLabel(qname string) string {
	i, shot := dns.NextLabel(qname, 0)
	if shot {
		return ""
	}

	return "*." + qname[i:]
}
