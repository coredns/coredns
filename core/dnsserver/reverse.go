package dnsserver

import (
	"math"
	"net"

	"github.com/apparentlymart/go-cidr/cidr"
)

// classFromCIDR return slice of "classful" (/8, /16, /24 or /32 only) CIDR's from the CIDR in net.
func classFromCIDR(n *net.IPNet) []string {
	ones, _ := n.Mask.Size()
	if ones%8 == 0 {
		return []string{n.String()}
	}

	mask := int(math.Ceil(float64(ones)/8)) * 8
	networks := subnets(n, mask)
	cidrs := make([]string, len(networks))
	for i := range networks {
		cidrs[i] = networks[i].String()
	}
	return cidrs
}

// subnets return a slice of prefixes with the desired mask subnetted from original network.
func subnets(network *net.IPNet, newPrefixLen int) []*net.IPNet {
	prefixLen, _ := network.Mask.Size()
	maxSubnets := int(math.Exp2(float64(newPrefixLen)) / math.Exp2(float64(prefixLen)))
	nets := []*net.IPNet{{network.IP, net.CIDRMask(newPrefixLen, 8*len(network.IP))}}

	for i := 1; i < maxSubnets; i++ {
		next, _ := cidr.NextSubnet(nets[len(nets)-1], newPrefixLen)
		nets = append(nets, next)
	}

	return nets
}
