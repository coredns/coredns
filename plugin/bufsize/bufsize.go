// Package bufsize implements a plugin that clamps EDNS0 buffer size preventing packet fragmentation.
package bufsize

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// Bufsize implements bufsize plugin.
type Bufsize struct {
	Next plugin.Handler
	Size int
}

// ServeDNS implements the plugin.Handler interface.
func (buf Bufsize) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if option := r.IsEdns0(); option != nil && int(option.UDPSize()) > buf.Size {
		option.SetUDPSize(uint16(buf.Size))
	}
	return plugin.NextOrFailure(buf.Name(), buf.Next, ctx, w, r)
}

// Name implements the Handler interface.
func (buf Bufsize) Name() string { return "bufsize" }
