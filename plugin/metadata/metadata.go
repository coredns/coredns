package metadata

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
)

const (
	// pluginName is the name of the plugin
	pluginName = "metadata"
)

// Metadata implements collecting metadata information from all plugins that
// implement the Provider interface.
// Also, it can write metadata for request based on specified mapping.
type Metadata struct {
	Zones     []string
	Providers []Provider
	Next      plugin.Handler

	// Map defines a mapping of request field to metadata.
	Map *Map
}

// ContextWithMetadata is exported for use by provider tests
func ContextWithMetadata(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, md{})
}

// Collect will retrieve metadata functions from each metadata provider and update the context
func (m *Metadata) Collect(ctx context.Context, state request.Request) context.Context {
	ctx = ContextWithMetadata(ctx)
	if plugin.Zones(m.Zones).Matches(state.Name()) != "" {
		// Go through all Providers and collect metadata.
		for _, p := range m.Providers {
			ctx = p.Metadata(ctx, state)
		}
	}
	return ctx
}
