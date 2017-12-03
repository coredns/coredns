// Package nsid implements NSID protocol
package nsid

import (
	"encoding/hex"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Nsid plugin
type Nsid struct {
	Next plugin.Handler
	Data string
}

// ResponseWriter is a response writer that adds NSID response
type ResponseWriter struct {
	dns.ResponseWriter
	Data string
}

// ServeDNS implements the plugin.Handler interface.
func (n Nsid) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if option := r.IsEdns0(); option != nil {
		for _, o := range option.Option {
			if _, ok := o.(*dns.EDNS0_NSID); ok {
				nw := &ResponseWriter{ResponseWriter: w, Data: n.Data}
				return plugin.NextOrFailure(n.Name(), n.Next, ctx, nw, r)

			}
		}
	}
	return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ResponseWriter) WriteMsg(res *dns.Msg) error {
	r1 := new(dns.Msg)

	// Make a copy of Msg and update NSID
	r1.MsgHdr = res.MsgHdr
	r1.Compress = res.Compress
	if len(res.Question) > 0 {
		r1.Question = make([]dns.Question, len(res.Question))
		copy(r1.Question, res.Question)
	}
	rrArr := make([]dns.RR, len(res.Answer)+len(res.Ns)+len(res.Extra))
	var rri int
	if len(res.Answer) > 0 {
		rrbegin := rri
		for i := 0; i < len(res.Answer); i++ {
			rrArr[rri] = dns.Copy(res.Answer[i])
			rri++
		}
		r1.Answer = rrArr[rrbegin:rri:rri]
	}
	if len(res.Ns) > 0 {
		rrbegin := rri
		for i := 0; i < len(res.Ns); i++ {
			rrArr[rri] = dns.Copy(res.Ns[i])
			rri++
		}
		r1.Ns = rrArr[rrbegin:rri:rri]
	}
	if len(res.Extra) > 0 {
		rrbegin := rri
		for i := 0; i < len(res.Extra); i++ {
			extra := res.Extra[i]
			if v, ok := extra.(*dns.OPT); ok {
				option := new(dns.OPT)
				option.Hdr.Name = v.Hdr.Name
				option.Hdr.Rrtype = v.Hdr.Rrtype
				for _, o := range v.Option {
					if _, ok := o.(*dns.EDNS0_NSID); ok {
						e := new(dns.EDNS0_NSID)
						e.Code = dns.EDNS0NSID
						e.Nsid = hex.EncodeToString([]byte(w.Data))
						o = e
					}
					option.Option = append(option.Option, o)
				}
				extra = option
			}
			rrArr[rri] = dns.Copy(extra)
			rri++
		}
		r1.Extra = rrArr[rrbegin:rri:rri]
	}

	return w.ResponseWriter.WriteMsg(r1)
}

// Name implements the Handler interface.
func (n Nsid) Name() string { return "nsid" }
