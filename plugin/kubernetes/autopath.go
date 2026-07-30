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

	totalSize := 4 + len(k.autoPathSearch)
	search := make([]string, totalSize)
	if zone == "." {
		search[0] = pod.Namespace + ".svc."
		search[1] = "svc."
		search[2] = "."
	} else {
		svcZone := "svc." + zone
		search[0] = pod.Namespace + "." + svcZone
		search[1] = svcZone
		search[2] = zone
	}

	copy(search[3:], k.autoPathSearch)
	search[totalSize-1] = "" // sentinel
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
	return ps[0]
}
