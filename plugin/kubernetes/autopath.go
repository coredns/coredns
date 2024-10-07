package kubernetes

import (
	"fmt"
	"regexp"
	"strings"

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
	fmt.Println("ip", ip, "zone", zone)

	var namespace string
	pods := k.podsWithIP(ip)
	if pods == nil {
		return nil
	}

	if len(pods) == 1 {
		namespace = pods[0].Namespace
	} else {
		var expr *regexp.Regexp
		nsMatcher := namespaceRegex(pods)
		if zone == "." {
			expr = regexp.MustCompile(fmt.Sprintf(`.+\.%s\.svc\.`, nsMatcher))
		} else {
			regexSafeZone := strings.ReplaceAll(zone, ".", "\\.")
			expr = regexp.MustCompile(fmt.Sprintf(`.+\.%s\.svc\.%s`, nsMatcher, regexSafeZone))
		}
		matches := expr.FindStringSubmatch(state.Name())
		//We are only matching <base-query>.<pod-namespace>.svc.<zone>
		//since we only want to trigger autopath on the first query
		if matches == nil {
			return nil
		}
		namespace = matches[1]
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

func namespaceRegex(pods []*object.Pod) string {
	nsMap := map[string]struct{}{}
	for _, pod := range pods {
		nsMap[pod.Namespace] = struct{}{}
	}
	var namespaces []string
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}
	return fmt.Sprintf("(%s)", strings.Join(namespaces, "|"))
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
