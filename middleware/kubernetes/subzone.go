package kubernetes

import (
    "github.com/miekg/dns"
)

func SubzoneConflict(zones []string, name string) (bool, []string) {
    /*
     * SubzoneConflict returns true if name is a subzone or parent zone of
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
