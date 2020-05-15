package iplookup

import (
	"context"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

type InterceptRecorder struct {
	dns.ResponseWriter
	ipl *IPLookup
}

func (ipl *IPLookup) NewInterceptRecorder(w dns.ResponseWriter) *InterceptRecorder {

	return &InterceptRecorder{
		ResponseWriter: w,
		ipl:            ipl,
	}

}

// WriteMsg records the status code and calls the
// underlying ResponseWriter's WriteMsg method.
func (ir *InterceptRecorder) WriteMsg(res *dns.Msg) error {

	lookup := make(map[string]string)
	alias := make(map[string]string)

	for _, ans := range res.Answer {
		switch rec := ans.(type) {
		case *dns.A:
			lookup[rec.A.String()] = rec.Hdr.Name
		case *dns.CNAME:
			alias[rec.Target] = rec.Hdr.Name
		}
	}

	// The rest of this can be done in the background so we don't delay the response
	go func() {

		// Recursively search aliases until we find the source of an IP lookup
		var findAlias func(name string) string
		findAlias = func(name string) string {
			if cname, found := alias[name]; found {
				return findAlias(cname)
			}
			return name
		}

		// Replace the name with an alias, store it in the cache
		for ip, name := range lookup {
			ir.ipl.addCache(ip, findAlias(name))
		}

	}()

	return ir.ResponseWriter.WriteMsg(res)
}

// ServeDNS implements the plugin.Handler interface.
func (ipl *IPLookup) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(ipl.Name(), ipl.Next, ctx, ipl.NewInterceptRecorder(w), r)
}
