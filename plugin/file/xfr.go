package file

import (
	"github.com/coredns/coredns/plugin/file/tree"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

// Transfer implements the transfer.Transfer interface.
func (f File) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	z, ok := f.Z[zone]
	if !ok || z == nil {
		return nil, transfer.ErrNotAuthoritative
	}
	return z.Transfer(serial)
}

// Transfer transfers a zone with serial in the returned channel and implements IXFR fallback, by just
// sending a single SOA record.
func (z *Zone) Transfer(serial uint32) (<-chan []dns.RR, error) {
	ap, t := z.snapshot()
	apex, err := ap.records()
	if err != nil {
		return nil, err
	}

	ch := make(chan []dns.RR)
	go func() {
		if serial != 0 && ap.SOA.Serial == serial { // ixfr fallback, only send SOA
			ch <- []dns.RR{ap.SOA}

			close(ch)
			return
		}

		ch <- apex
		t.Walk(func(e *tree.Elem, _ map[uint16][]dns.RR) error { ch <- e.All(); return nil })
		ch <- []dns.RR{ap.SOA}

		close(ch)
	}()

	return ch, nil
}
