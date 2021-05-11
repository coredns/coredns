// Package cidr contains functions that deal with classless reverse zones in the DNS.
package cidr

import (
	"math"
	"net"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/miekg/dns"
)

// Class return slice of "classful" (/8, /16, /24 or /32 only) CIDR's from the CIDR in net.
func Class(n *net.IPNet) []string {
	ones, _ := n.Mask.Size()
	if ones%8 == 0 {
		return []string{n.String()}
	}

	mask := int(math.Ceil(float64(ones)/8)) * 8
	networks := nets(n, mask)
	cidrs := make([]string, len(networks))
	for i := range networks {
		cidrs[i] = networks[i].String()
	}
	return cidrs
}

// nets return a slice of prefixes with the desired mask subnetted from original network.
func nets(network *net.IPNet, newPrefixLen int) []*net.IPNet {
	prefixLen, _ := network.Mask.Size()
	maxSubnets := int(math.Exp2(float64(newPrefixLen)) / math.Exp2(float64(prefixLen)))
	nets := []*net.IPNet{{network.IP, net.CIDRMask(newPrefixLen, 8*len(network.IP))}}

	for i := 1; i < maxSubnets; i++ {
		next, exceeds := cidr.NextSubnet(nets[len(nets)-1], newPrefixLen)
		nets = append(nets, next)
		if exceeds {
			break
		}
	}

	return nets
}

// Reverse return the reverse zones that are authoritative for each net in ns.
func Reverse(nets []string) []string {
	rev := make([]string, len(nets))
	for i := range nets {
		ip, n, _ := net.ParseCIDR(nets[i])
		r, err1 := dns.ReverseAddr(ip.String())
		if err1 != nil {
			continue
		}
		ones, bits := n.Mask.Size()
		// get the size, in bits, of each portion of hostname defined in the reverse address. (8 for IPv4, 4 for IPv6)
		sizeDigit := 8
		if len(n.IP) == net.IPv6len {
			sizeDigit = 4
		}
		// Get the first lower octet boundary to see what encompassing zone we should be authoritative for.
		mod := (bits - ones) % sizeDigit
		nearest := (bits - ones) + mod
		offset := 0
		var end bool
		for i := 0; i < nearest/sizeDigit; i++ {
			offset, end = dns.NextLabel(r, offset)
			if end {
				break
			}
		}
		rev[i] = r[offset:]
	}
	return rev
}
