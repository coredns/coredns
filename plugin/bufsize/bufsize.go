package bufsize

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type Bufsize struct {
	Next plugin.Handler
	Size int
}

// ServeDNS implements the plugin.Handler interface.
func (buf Bufsize) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	a := new(dns.Msg)
	a.SetReply(r)
	a.Authoritative = true

	w.WriteMsg(a)

	return 0, nil
}

// Name implements the Handler interface.
func (buf Bufsize) Name() string { return "bufsize" }
