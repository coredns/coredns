package fanout

import (
	"github.com/miekg/dns"
	"time"
)

type connectResult struct {
	client   Client
	response *dns.Msg
	start    time.Time
	err      error
}

func isBetter(left, right *connectResult) bool {
	if right == nil {
		return false
	}
	if left == nil {
		return true
	}
	if right.err != nil {
		return false
	}
	if left.err != nil {
		return true
	}
	if right.response == nil {
		return false
	}
	if left.response == nil {
		return true
	}
	return left.response.MsgHdr.Rcode != dns.RcodeSuccess &&
		right.response.MsgHdr.Rcode == dns.RcodeSuccess
}
