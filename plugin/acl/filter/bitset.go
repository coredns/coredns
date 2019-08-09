package filter

import (
	"encoding/binary"
	"net"

	"github.com/willf/bitset"
)

// bitsetFilter maps every IPv4 address to a unique unsigned integer and allows
// to assign a binary flag (block or allow) on it.
// bitsetFilter is a constant time filtering technique.
// Note: subnet is not supported.
type bitsetFilter struct {
	bitset bitset.BitSet
	subnets []net.IPNet
}

var _ Filter = &bitsetFilter{}

func (bsf *bitsetFilter) Add(subnet net.IPNet) error {
	if isSingleIP(subnet) {
		ipNum := uint(encode(subnet.IP))
		bsf.bitset.Set(ipNum)
	} else {
		bsf.subnets = append(bsf.subnets, subnet)
	}
	return nil
}

func (bsf *bitsetFilter) Contains(ip net.IP) bool {
	ipNum := uint(encode(ip))
	if bsf.bitset.Test(ipNum) {
		return true
	}
	for _, subnet := range bsf.subnets {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func newBitSetFilter(subnets []net.IPNet) (*bitsetFilter, error) {
	bsf := bitsetFilter{}
	for _, subnet := range subnets {
		err := bsf.Add(subnet)
		if err != nil {
			return nil, err
		}
	}
	return &bsf, nil
}

// encode converts an ip address (only IPv4 is supported currently) to
// an unsigned integer.
func encode(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}
