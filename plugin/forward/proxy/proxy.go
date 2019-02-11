package proxy

import (
	"context"
	"time"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Proxy defines an upstream host.
type Proxy interface {
	Query(ctx context.Context, state request.Request) (*dns.Msg, error)
	Healthcheck()
	Down(maxfails uint32) bool
	Stop()
	Start(duration time.Duration)
	Address() string
	FailsNum() *uint32
}
