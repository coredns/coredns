package filter

import (
	"fmt"
	"net"
)

// Filter allows to whether an IP address is present in
// a set of IP addresses or subnets (in CIDR notation).
type Filter interface {
	Add(net.IPNet) error
	Contains(net.IP) bool
}

// New creates a Filter.
func New(filterType string, subnets []net.IPNet) (Filter, error) {
	switch filterType {
	case "naive":
		return newNaiveFilter(subnets)
	case "cuckoo":
		return newCuckooFilter(subnets)
	case "trie":
		return newTrieFilter(subnets)
	case "bitset":
		return newBitSetFilter(subnets)
	default:
		return nil, fmt.Errorf("unrecognized filter type: %s", filterType)
	}
}
