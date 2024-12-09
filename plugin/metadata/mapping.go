package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	// defaultKey is the key for default value.
	defaultKey = "default"
	// errorLabel is a metadata label for error that can be occurred in Metadata method.
	errorLabel = pluginName + "/error"
)

type Map struct {
	// Source is the source of key for Mapping
	Source *template.Template
	// Mapping maps source value to metadata entries
	Mapping map[string][]KeyValue
	// RcodeOnMiss defined a behavior what rcode is returns if mapping is not found for the source.
	// By default, metadata plugin do not set any metadata and just call a next plugin.
	RcodeOnMiss int
}

// KeyValue structure that stores single metadata.
type KeyValue struct {
	Key   string
	Value string
}

// Name implements the Handler interface.
func (m *Metadata) Name() string { return pluginName }

// ServeDNS implements the plugin.Handler interface.
// Returns an error if no mapping is found, otherwise just call next plugin.
func (m *Metadata) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if errMsg := ValueFunc(ctx, errorLabel); errMsg != nil {
		return m.Map.RcodeOnMiss, errors.New(errMsg())
	}
	return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)
}

// Metadata implements the metadata.Provider interface.
// Resolves source from DNS request, finds specific metadata values and adds them to the context.
func (m *Metadata) Metadata(ctx context.Context, state request.Request) context.Context {
	if m.Map == nil {
		return ctx
	}

	k := bytes.Buffer{}
	if err := m.Map.Source.Execute(&k, &state); err != nil {
		return setError(ctx, fmt.Sprintf("unable to resolve %s: %v", m.Map.Source.Name(), err))
	}

	// try to find by source value
	if metadata, found := m.Map.Mapping[k.String()]; found {
		return setMetadata(ctx, metadata)
	}

	// if not found - use default
	if metadata, found := m.Map.Mapping[defaultKey]; found {
		return setMetadata(ctx, metadata)
	}

	// return error if Rcode is expected on miss
	if m.Map.RcodeOnMiss != 0 {
		return setError(ctx, fmt.Sprintf("no values found for %s '%s'", m.Map.Source.Name(), k.String()))
	}

	return ctx
}

// Sets metadata entries to context using metadata.SetValueFunc.
func setMetadata(ctx context.Context, md []KeyValue) context.Context {
	for _, meta := range md {
		SetValueFunc(ctx, fmt.Sprintf("%s/%s", pluginName, meta.Key), func() string {
			return meta.Value
		})
	}
	return ctx
}

// Sets error to context using metadata.SetValueFunc.
func setError(ctx context.Context, errMsg string) context.Context {
	SetValueFunc(ctx, errorLabel, func() string {
		return errMsg
	})
	return ctx
}
