package filter

import "net"

// naiveFilter implements Filter by sequentially traversing the pre-configured ip set and
// matching them with the source ip.
type naiveFilter struct {
	subnets []net.IPNet
}

var _ Filter = &naiveFilter{}

func (nf *naiveFilter) Add(subnet net.IPNet) error {
	nf.subnets = append(nf.subnets, subnet)
	return nil
}

func (nf *naiveFilter) Contains(ip net.IP) bool {
	for _, subnet := range nf.subnets {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func newNaiveFilter(subnets []net.IPNet) (*naiveFilter, error) {
	return &naiveFilter{
		subnets: subnets,
	}, nil
}
