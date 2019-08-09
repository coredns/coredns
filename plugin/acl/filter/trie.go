package filter

import (
	"encoding/binary"
	"net"
)

// trieFilter utilizes trie (tree-based data structure) to store and match ip addresses.
// Subnet is well supported in trieFilter.
type trieFilter struct {
	trie *trieNode
}

type trieNode struct {
	zero   *trieNode
	one    *trieNode
	isLeaf bool
}

func insert(root *trieNode, subnet net.IPNet) {
	// skip IPv6.
	if len(subnet.IP.To4()) == 0 {
		return
	}
	ip := binary.BigEndian.Uint32(subnet.IP.To4())
	mask := binary.BigEndian.Uint32(subnet.Mask)
	curr := root
	for {
		if mask == 0 {
			curr.isLeaf = true
			return
		}
		if (ip & 0x80000000) != 0 {
			if curr.one == nil {
				curr.one = &trieNode{}
			}
			curr = curr.one
		} else {
			if curr.zero == nil {
				curr.zero = &trieNode{}
			}
			curr = curr.zero
		}
		ip <<= 1
		mask <<= 1
	}
}

func find(root *trieNode, ip net.IP) bool {
	ipNum := binary.BigEndian.Uint32(ip.To4())
	curr := root
	for curr != nil {
		if curr.isLeaf {
			return true
		}
		if (ipNum & 0x80000000) != 0 {
			curr = curr.one
		} else {
			curr = curr.zero
		}
		ipNum <<= 1
	}
	return false
}

var _ Filter = &trieFilter{}

func (tf *trieFilter) Add(subnet net.IPNet) error {
	insert(tf.trie, subnet)
	return nil
}

func (tf *trieFilter) Contains(ip net.IP) bool {
	return find(tf.trie, ip)
}

func newTrieFilter(subnets []net.IPNet) (*trieFilter, error) {
	trie := &trieNode{}
	for _, subnet := range subnets {
		insert(trie, subnet)
	}
	return &trieFilter{
		trie: trie,
	}, nil
}
