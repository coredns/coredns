package file

import (
	"github.com/coredns/coredns/plugin/file/tree"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
)

// Transfer implements Transfer.Transferer
func (f File) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	// look for exact zone match
	var z *Zone
	for fz, zo := range f.Z {
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

// Transfer returns a channel containing records of a zone transfer response for the zone
func (z Zone) Transfer(serial uint32) (<-chan []dns.RR, error) {
	ch := make(chan []dns.RR, 2)

	// get soa and apex
	apex, err := z.ApexIfDefined()
	if err != nil {
		close(ch)
		return nil, err
	}

	ch <- apex

	// ApexIfDefined ensures that first record is an SOA
	if serial >= apex[0].(*dns.SOA).Serial && serial != 0 {
		// Zone is up to date. Just return the SOA
		close(ch)
		return ch, nil
	}

	go func() {
		z.Walk(func(e *tree.Elem, _ map[uint16][]dns.RR) error {
			ch <- e.All()
			return nil
		})
		close(ch)
	}()

	return ch, nil
}
