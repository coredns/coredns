package kubernetes

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
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

	var namespace string
	pods := k.podsWithIP(ip)
	if pods == nil {
		return nil
	}

	if len(pods) == 1 {
		namespace = pods[0].Namespace
	} else {
		matchedPods := k.matchQuery(pods, state.Name(), zone)
		//If we can't find a match the namespace must be missing in the query, so this isn't the first query (searchpath <ns>.svc.<zone>)
		//In this case we can return nil since autopath is only done on the first query.
		if len(matchedPods) == 0 {
			return nil
		}
		namespace = matchedPods[0].Namespace
	}

	search := make([]string, 3)
	if zone == "." {
		search[0] = namespace + ".svc."
		search[1] = "svc."
		search[2] = "."
	} else {
		search[0] = namespace + ".svc." + zone
		search[1] = "svc." + zone
		search[2] = zone
	}

	search = append(search, k.autoPathSearch...)
	search = append(search, "") // sentinel
	return search
}

// matchQuery only returns pods where the pod namespace matches the query namespace (if the query is <ns>.svc.<zone>)
func (k *Kubernetes) matchQuery(pods []*object.Pod, query string, zone string) []*object.Pod {
	zoneLength := dns.CountLabel(zone)
	queryParts := dns.SplitDomainName(query)
	// The namespace is always at the position before "svc", the second to last label before the zone.
	namespace := queryParts[len(queryParts)-zoneLength-2]
	var matchedPods []*object.Pod
	for _, pod := range pods {
		if pod.Namespace == namespace {
			matchedPods = append(matchedPods, pod)
		}
	}
	return matchedPods
}

// podsWithIP returns the list of api.Pod for source IP. It returns nil if nothing can be found.
// Return a list because there can be multiple host network pods with the same IP (node IP).
func (k *Kubernetes) podsWithIP(ip string) []*object.Pod {
	if k.podMode != podModeVerified {
		return nil
	}
	ps := k.APIConn.PodIndex(ip)
	if len(ps) == 0 {
		return nil
	}
	return ps
}
