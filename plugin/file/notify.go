package file

import (
	"net"
	"net/netip"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// isNotify checks if state is a notify message and if so, will *also* check if it
// is from one of the configured masters. If not it will not be a valid notify
// message. If the zone z is not a secondary zone the message will also be ignored.
func (z *Zone) isNotify(state request.Request) bool {
	if state.Req.Opcode != dns.OpcodeNotify {
		return false
	}
	if len(z.TransferFrom) == 0 {
		return false
	}
	// If remote IP matches we accept.
	remote, err := netip.ParseAddr(state.IP())
	if err != nil {
		return false
	}
	remote = remote.Unmap()
	for _, f := range z.TransferFrom {
		from, _, err := net.SplitHostPort(f)
		if err != nil {
			continue
		}
		addr, err := netip.ParseAddr(from)
		if err != nil {
			continue
		}
		if addr.Unmap() == remote {
			return true
		}
	}
	return false
}
