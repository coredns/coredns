package file

import (
	"net"

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
	remote := state.IP()
	for _, f := range z.TransferFrom {
		from, _, err := net.SplitHostPort(f)
		if err != nil {
			continue
		}
		if from == remote {
			return true
		}
	}
	return false
}
