package kubernetes

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/request"
)

// AutoPath implements the AutoPathFunc call from the autopath plugin.
// It returns a per-query search path or nil indicating no searchpathing should happen.
func (k *Kubernetes) AutoPath(state request.Request) []string {
	// Check if the query falls in a zone we are actually authoritative for and thus if we want autopath.
	zone := plugin.Zones(k.Zones).Matches(state.Name())
	if zone == "" {
		return nil
	}

	// cluster.local {
	//    autopath @kubernetes
	//    kubernetes {
	//        pods verified #
	//    }
	// }
	// if pods != verified will cause panic and return SERVFAIL, expect worked as normal without autopath function
	if !k.opts.initPodCache {
		return nil
	}

	ip := state.IP()

	pod := k.podWithIP(ip)
	if pod == nil {
		return nil
	}

	totalSize := 3 + len(k.autoPathSearch) + 1 // +1 for sentinel
	search := make([]string, 0, totalSize)
	if zone == "." {
		search = append(search, pod.Namespace+".svc.", "svc.", ".")
	} else {
		search = append(search, pod.Namespace+".svc."+zone, "svc."+zone, zone)
	}

	search = append(search, k.autoPathSearch...)
	search = append(search, "") // sentinel
	return search
}

// podWithIP returns the api.Pod for source IP. It returns nil if nothing can be found.
func (k *Kubernetes) podWithIP(ip string) *object.Pod {
	if k.podMode != podModeVerified {
		return nil
	}
	ps := k.APIConn.PodIndex(ip)
	if len(ps) == 0 {
		return nil
	}
	if ps[0].HostNetwork {
		return nil
	}
	return ps[0]
}
