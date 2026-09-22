// Package catalog parses DNS catalog zones as defined by RFC 9432.
package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// Version is the RFC 9432 catalog zone schema version supported by this package.
const Version = "2"

// Catalog is the parsed catalog zone state.
type Catalog struct {
	Origin  string
	Members []Member
}

// Member is a member zone entry from a catalog zone.
type Member struct {
	ID                string
	Zone              string
	Groups            []string
	ChangeOfOwnership string
}

// Parse builds a Catalog from resource records belonging to origin.
func Parse(origin string, rrs []dns.RR) (*Catalog, error) {
	origin = normalizeName(origin)
	originLabels := dns.SplitDomainName(origin)
	zonesLabels := append([]string{"zones"}, originLabels...)
	versionOwner := "version." + origin

	var (
		hasSOA      bool
		hasNS       bool
		versionText []string
	)

	memberPTR := make(map[string][]string)
	groups := make(map[string][]string)
	coo := make(map[string][]string)

	for _, rr := range rrs {
		h := rr.Header()
		if h.Class != dns.ClassINET {
			return nil, fmt.Errorf("catalog zone %s contains non-IN record %s", origin, rr.String())
		}

		owner := normalizeName(h.Name)
		switch x := rr.(type) {
		case *dns.SOA:
			if owner == origin {
				hasSOA = true
			}
		case *dns.NS:
			if owner == origin {
				hasNS = true
			}
		case *dns.TXT:
			if owner == versionOwner {
				versionText = append(versionText, txtValue(x))
			} else {
				if prop, id, ok := propertyOwner(owner, zonesLabels); ok && prop == "group" {
					groups[id] = append(groups[id], txtValue(x))
				}
			}
		case *dns.PTR:
			switch {
			case isMemberOwner(owner, zonesLabels):
				id := dns.SplitDomainName(owner)[0]
				memberPTR[id] = append(memberPTR[id], normalizeName(x.Ptr))
			default:
				if prop, id, ok := propertyOwner(owner, zonesLabels); ok && prop == "coo" {
					coo[id] = append(coo[id], normalizeName(x.Ptr))
				}
			}
		}
	}

	if !hasSOA {
		return nil, fmt.Errorf("catalog zone %s has no SOA record", origin)
	}
	if !hasNS {
		return nil, fmt.Errorf("catalog zone %s has no NS record", origin)
	}
	if len(versionText) != 1 {
		return nil, fmt.Errorf("catalog zone %s must have exactly one version TXT record", origin)
	}
	if versionText[0] != Version {
		return nil, fmt.Errorf("catalog zone %s has unsupported version %q", origin, versionText[0])
	}

	seenZones := make(map[string]string)
	members := make([]Member, 0, len(memberPTR))
	for id, zones := range memberPTR {
		if len(zones) != 1 {
			return nil, fmt.Errorf("catalog member %s.%s must have exactly one PTR record", id, "zones."+origin)
		}
		zone := zones[0]
		if prevID, ok := seenZones[zone]; ok {
			return nil, fmt.Errorf("catalog member zone %s is listed by both %s and %s", zone, prevID, id)
		}
		seenZones[zone] = id

		member := Member{ID: id, Zone: zone, Groups: append([]string(nil), groups[id]...)}
		sort.Strings(member.Groups)
		if values := coo[id]; len(values) > 1 {
			return nil, fmt.Errorf("catalog member %s has more than one coo PTR record", id)
		} else if len(values) == 1 {
			member.ChangeOfOwnership = values[0]
		}
		members = append(members, member)
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].ID < members[j].ID
	})

	return &Catalog{Origin: origin, Members: members}, nil
}

func normalizeName(name string) string {
	return strings.ToLower(dns.Fqdn(name))
}

func txtValue(rr *dns.TXT) string {
	return strings.Join(rr.Txt, "")
}

func isMemberOwner(owner string, zonesLabels []string) bool {
	labels := dns.SplitDomainName(owner)
	return len(labels) == len(zonesLabels)+1 && hasLabelSuffix(labels, zonesLabels)
}

func propertyOwner(owner string, zonesLabels []string) (property, id string, ok bool) {
	labels := dns.SplitDomainName(owner)
	if len(labels) != len(zonesLabels)+2 || !hasLabelSuffix(labels, zonesLabels) {
		return "", "", false
	}
	return labels[0], labels[1], true
}

func hasLabelSuffix(labels, suffix []string) bool {
	if len(labels) < len(suffix) {
		return false
	}
	offset := len(labels) - len(suffix)
	for i := range suffix {
		if labels[offset+i] != suffix[i] {
			return false
		}
	}
	return true
}
