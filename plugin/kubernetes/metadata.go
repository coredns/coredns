package kubernetes

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

// Metadata implements the metadata.Provider interface.
func (k *Kubernetes) Metadata(ctx context.Context, state request.Request) context.Context {
	pod := k.podWithIP(state.IP())
	if pod != nil {
		metadata.SetValueFunc(ctx, "kubernetes/client-namespace", func() string {
			return pod.Namespace
		})

		metadata.SetValueFunc(ctx, "kubernetes/client-pod-name", func() string {
			return pod.Name
		})
	}

	zone := plugin.Zones(k.Zones).Matches(state.Name())
	if zone == "" {
		return ctx
	}
	// possible optimization: cache r so it doesn't need to be calculated again in ServeDNS
	r, err := parseRequest(state.Name(), zone)
	if err != nil {
		metadata.SetValueFunc(ctx, "kubernetes/parse-error", func() string {
			return err.Error()
		})
		return ctx
	}

	metadata.SetValueFunc(ctx, "kubernetes/port-name", func() string {
		return r.port
	})

	metadata.SetValueFunc(ctx, "kubernetes/protocol", func() string {
		return r.protocol
	})

	metadata.SetValueFunc(ctx, "kubernetes/endpoint", func() string {
		return r.endpoint
	})

	metadata.SetValueFunc(ctx, "kubernetes/service", func() string {
		return r.service
	})

	metadata.SetValueFunc(ctx, "kubernetes/namespace", func() string {
		return r.namespace
	})

	metadata.SetValueFunc(ctx, "kubernetes/kind", func() string {
		return r.podOrSvc
	})

	return ctx
}
