// Package upstream abstracts a upstream lookups so that plugins can handle them in an unified way.
package upstream

import (
	"fmt"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"
)

// Upstream is used to resolve CNAME or other external targets via CoreDNS itself.
type Upstream struct{}

// New creates a new Upstream to resolve names using the coredns process.
func New() *Upstream { return &Upstream{} }

// Lookup routes lookups to our selves or forward to a remote.
func (u *Upstream) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	req := new(dns.Msg)
	req.SetQuestion(name, typ)

	nw := nonwriter.New(state.W)
	server, ok := state.Context.Value(dnsserver.Key{}).(*dnsserver.Server)
	if !ok {
		// this is so it can be used in template_test unittest
		handler, handlerOk := state.Context.Value(dnsserver.Key{}).(plugin.Handler)
		if !handlerOk {
			return nil, fmt.Errorf("no full server is running")
		}
		handler.ServeDNS(state.Context, nw, req)
		return nw.Msg, nil
	}

	server.ServeDNS(state.Context, nw, req)

	return nw.Msg, nil
}
