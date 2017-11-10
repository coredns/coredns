package dnstap

import (
	"time"

	"github.com/coredns/coredns/request"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// ToMessage converts a state and possible reply to a dnstap message.
func ToMessage(ctx context.Context, upstream, proto string, state request.Request, reply *dns.Msg, start time.Time) error {
	tapper := TapperFromContext(ctx)
	if tapper == nil {
		return nil
	}

	// Query
	b := tapper.TapBuilder()
	b.TimeSec = uint64(start.Unix())
	if err := b.HostPort(upstream); err != nil {
		return err
	}

	if proto == "tcp" {
		b.SocketProto = tap.SocketProtocol_TCP
	} else {
		b.SocketProto = tap.SocketProtocol_UDP
	}

	if err := b.Msg(state.Req); err != nil {
		return err
	}

	if err := tapper.TapMessage(b.ToOutsideQuery(tap.Message_FORWARDER_QUERY)); err != nil {
		return err
	}

	// Response
	if reply != nil {
		b.TimeSec = uint64(time.Now().Unix())
		if err := b.Msg(reply); err != nil {
			return err
		}
		return tapper.TapMessage(b.ToOutsideResponse(tap.Message_FORWARDER_RESPONSE))
	}
	return nil
}
