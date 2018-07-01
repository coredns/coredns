package metadata

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/variables"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Metadata implements collecting metadata information from all plugins that
// implement the Provider interface.
type Metadata struct {
	Zones     []string
	Providers []Provider
	Next      plugin.Handler
}

// Name implements the Handler interface.
func (m *Metadata) Name() string { return "metadata" }

// ServeDNS implements the plugin.Handler interface.
func (m *Metadata) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	ctx = context.WithValue(ctx, metadataKey{}, M{})
	md, _ := FromContext(ctx)

	state := request.Request{W: w, Req: r}
	if plugin.Zones(m.Zones).Matches(state.Name()) != "" {
		// Go through all Providers and collect metadata.
		for _, provider := range m.Providers {
			for _, v := range provider.VarStrings() {
				val := provider.String(ctx, state, v)
				if val != "" {
					md.SetString(v, val)
				}
			}
			for _, v := range provider.VarInts() {
				val := provider.Int(ctx, state, v)
				if val != -1 {
					md.SetInt(v, val)
				}
			}
		}
	}

	rcode, err := plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)

	return rcode, err
}

// Metadata implements the plugin.Provider interface.
func (m *Metadata) String(ctx context.Context, state request.Request, v string) string {
	return variables.StringValue(state, v)
}

// Metadata implements the plugin.Provider interface.
func (m *Metadata) Int(ctx context.Context, state request.Request, v string) int {
	return variables.IntValue(state, v)
}
