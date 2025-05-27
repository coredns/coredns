package kubernetes

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

// Metadata implements the metadata.Provider interface.
func (k *Kubernetes) Metadata(ctx context.Context, state request.Request) context.Context {
	pods := k.podsWithIP(state.IP())
	zone := plugin.Zones(k.Zones).Matches(state.Name())
	if pods != nil {
		pod := pods[0]
		if len(pods) > 1 {
			matchedPods := k.matchQuery(pods, state.Name(), zone)
			// If there's multiple host network pods in the same namespace we can't determine which one did the query
			// Return the first one to keep the existing behavior
			if len(matchedPods) > 0 {
				pod = matchedPods[0]
			}
		}
		metadata.SetValueFunc(ctx, "kubernetes/client-namespace", func() string {
			return pod.Namespace
		})

		metadata.SetValueFunc(ctx, "kubernetes/client-pod-name", func() string {
			return pod.Name
		})

		for k, v := range pod.Labels {
			v := v
			metadata.SetValueFunc(ctx, "kubernetes/client-label/"+k, func() string {
				return v
			})
		}
	}

	if zone == "" {
		return ctx
	}
	multicluster := false
	if z := plugin.Zones(k.opts.multiclusterZones).Matches(state.Zone); z != "" {
		multicluster = true
	}
	// possible optimization: cache r so it doesn't need to be calculated again in ServeDNS
	r, err := parseRequest(state.Name(), zone, multicluster)
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

	if multicluster {
		metadata.SetValueFunc(ctx, "kubernetes/cluster", func() string {
			return r.cluster
		})
	}

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
