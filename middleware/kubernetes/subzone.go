package kubernetes

import (
	"fmt"

    "github.com/miekg/dns"
)

func NormalizeZoneList(zones []string) []string {
	/*
	 * Filter zones argument to remove any subzones from
	 * the zones argument.
	 * For example, providing the following zones array:
	 *    [ "a.b.c", "b.c", "a", "e.d.f", "a.b" ]
	 * will return:
	 *    [ "a.b.c", "a", "e.d.f", "a.b" ]
	 * The zones were filted out:
	 *    - "b.c" because "a.b.c" and "b.c" share the common top 
	 *      level "b.c". First defined zone wins if there is an overlap.
	 * 
	 * Note: This may prove to be too restrictive in practice.
	 *       Need to find coutner-example use-cases.
	 */

	filteredZones := []string{}

	for _, z := range zones {
		zoneConflict, _ := subzoneConflict(filteredZones, z)
		if zoneConflict {
			fmt.Printf("[WARN] new zone '%v' from Corefile conflicts with existing zones: %v\n", z, filteredZones)
			fmt.Printf("        Ignoring zone '%v'\n", z)
		} else {
			filteredZones = append(filteredZones, z)
		}
	}

	return filteredZones
}


func subzoneConflict(zones []string, name string) (bool, []string) {
    /*
     * SubzoneConflict returns true if name is a child or parent zone of
     * any element in zones. If conflicts exist, return the conflicting zones.
     */

    conflicts := []string{}

    for _, z := range zones {
        if dns.IsSubDomain(z, name) || dns.IsSubDomain(name, z) {
            conflicts = append(conflicts, z)
        }
    }

    return (len(conflicts) != 0), conflicts
}
