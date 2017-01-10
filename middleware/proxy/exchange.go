package proxy

import (
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
)

type Exchanger interface {
	Exchange(request.Request, *UpstreamHost) (*dns.Msg, error)
}
