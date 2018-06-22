package metadata

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Metadater interface needs to be implemented by each plugin willing to provide
// metadata information for other plugins.
// Note: this method should work quickly, because it is called for every request
// from the metadata plugin.
type Metadater interface {
	// Metadata gets content, ResponseWriter and dns.Msg and returns context with
	// additional values. Metadata must be thread safe.
	Metadata(context.Context, dns.ResponseWriter, *dns.Msg) (context.Context, error)
}

// Metadata implements collecting metadata information from all enabled plugins
// which provide it
type Metadata struct {
	Zones      []string
	Metadaters []Metadater
	Next       plugin.Handler
}

// Name implements the Handler interface.
func (m *Metadata) Name() string { return "metadata" }

// ServeDNS implements the plugin.Handler interface.
func (m *Metadata) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}
	if len(m.Zones) == 0 || plugin.Zones(m.Zones).Matches(state.Name()) != "" {
		// Go through all metadaters and collect metadata
		for _, metadater := range m.Metadaters {
			if c, err := metadater.Metadata(ctx, w, r); err == nil {
				ctx = c
			}
		}
	}

	rcode, err := plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)

	return rcode, err
}
