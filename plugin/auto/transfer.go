package auto

import (
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
)
// Transfer implements Transfer.Transferer
func (a Auto) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	// look for exact zone match
	var z *file.Zone
	for fz, zo := range a.Z {
		if zone == fz {
			z = zo
			break
		}
	}
	if z == nil {
		return nil, transfer.ErrNotAuthoritative
	}
	return z.Transfer(serial)
}
