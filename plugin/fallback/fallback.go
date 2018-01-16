package fallback

import (
	"log"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

type processor struct {
	rcode    int
	endpoint string
}

func (p *processor) ErrorFunc(w dns.ResponseWriter, r *dns.Msg, rc int) error {
	if rc == p.rcode {
		state := request.Request{W: w, Req: r}
		qname := state.Name()
		log.Printf("[INFO] Send fallback %q to %q", qname, p.endpoint)
		_, err := dns.Exchange(r, p.endpoint)
		return err
	}
	return nil
}

// Fallback is a plugin that provide fallback in case of error
type Fallback struct {
	Next  plugin.Handler
	zones []string
	funcs []processor
}

// ServeDNS implements the plugin.Handler interface.
func (f Fallback) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
}

// Name implements the Handler interface.
func (f Fallback) Name() string { return "fallback" }
