package ecs

import (
	"context"
	"net"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

// Ecs is an example plugin to show how to write a plugin.
type Ecs struct {
	Next   plugin.Handler
	v4Mask net.IPMask
	v6Mask net.IPMask

	v4MaskSize uint8
	v6MaskSize uint8
}

// setupEdns0Opt will retrieve the EDNS0 OPT or create it if it does not exist.
func setupEdns0Opt(r *dns.Msg) *dns.OPT {
	o := r.IsEdns0()
	if o == nil {
		r.SetEdns0(4096, false)
		o = r.IsEdns0()
	}
	return o
}

// ServeDNS implements the plugin.Handler interface. This method gets called when example is used
// in a Server.
func (e *Ecs) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	o := setupEdns0Opt(r)

	// If we already have an ECS option, skip
	for _, s := range o.Option {
		if _, ok := s.(*dns.EDNS0_SUBNET); ok {
			return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
		}
	}

	var srcOrig net.IP
	ip := w.RemoteAddr()
	if i, ok := ip.(*net.UDPAddr); ok {
		srcOrig = i.IP
	}
	if i, ok := ip.(*net.TCPAddr); ok {
		srcOrig = i.IP
	}

	if srcOrig.IsPrivate() {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	tmp4 := srcOrig.To4()
	// Skip 127.0.0.0/8
	if tmp4 != nil && tmp4[0] == 127 {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	ecs := &dns.EDNS0_SUBNET{Code: dns.EDNS0SUBNET}
	o.Option = append(o.Option, ecs)

	if tmp4 != nil {
		ecs.Family = 1
		ecs.Address = srcOrig.Mask(e.v4Mask)
		ecs.SourceNetmask = e.v4MaskSize
	} else {
		ecs.Family = 2
		ecs.Address = srcOrig.Mask(e.v6Mask)
		ecs.SourceNetmask = e.v6MaskSize
	}
	ecs.SourceScope = 0

	return plugin.NextOrFailure(e.Name(), e.Next, ctx, &ecsWriter{w}, r)
}

// Name implements the Handler interface.
func (e *Ecs) Name() string { return "ecs" }

// ecsWriter removes the ECS option from responses to requests that DID NOT originally include one
// See https://www.rfc-editor.org/rfc/rfc7871#section-7.2.2
type ecsWriter struct {
	dns.ResponseWriter
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ecsWriter) WriteMsg(res *dns.Msg) error {
	// Remove ECS option

	o := res.IsEdns0()
	if o != nil {
		for k, s := range o.Option {
			if _, ok := s.(*dns.EDNS0_SUBNET); ok {
				o.Option[k] = o.Option[len(o.Option)-1]
				o.Option = o.Option[:len(o.Option)-1]
				break
			}
		}
	}

	return w.ResponseWriter.WriteMsg(res)
}
