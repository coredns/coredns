// Package upstream abstracts a upstream lookups so that plugins can handle them in an unified way.
package upstream

import (
	"context"
	"fmt"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Upstream is used to resolve CNAME or other external targets via CoreDNS itself.
type Upstream struct{}

// New creates a new Upstream to resolve names using the coredns process.
func New() *Upstream { return &Upstream{} }

// Lookup routes lookups to our selves or forward to a remote.
func (u *Upstream) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	server, ok := ctx.Value(dnsserver.Key{}).(*dnsserver.Server)
	if !ok {
		return nil, fmt.Errorf("no full server is running")
	}

	size := state.Size()
	do := state.Do()
	req := new(dns.Msg)
	req.SetQuestion(name, typ)
	req.SetEdns0(uint16(size), do)

	nw := nonwriter.New(state.W)
	server.ServeDNS(ctx, nw, req)

	return nw.Msg, nil
}
