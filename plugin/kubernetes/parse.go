package kubernetes

import (
	"strings"

	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/miekg/dns"
)

type recordRequest struct {
	// The named port from the kubernetes DNS spec, this is the service part (think _https) from a well formed
	// SRV record.
	port string
	// The protocol is usually _udp or _tcp (if set), and comes from the protocol part of a well formed
	// SRV record.
	protocol string
	endpoint string
	cluster  string
	// The topology zone from a zone-scoped name (zone._zone.service.namespace.svc.zone);
	// only set when the zonal option is enabled.
	zone string
	// The servicename used in Kubernetes.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// zoneLabel marks a zone-scoped name: topozone._zone.service.namespace.svc.zone.
// The underscore keeps the label out of every hostname-shaped grammar: it cannot
// be an endpoint hostname, a pod name, or a multicluster cluster id, and in the
// _port._protocol position it reads as protocol "zone", which no Service port
// can carry (protocol is the TCP/UDP/SCTP enum).
const zoneLabel = "_zone"

// parseRequest parses the qname to find all the elements we need for querying k8s. Anything
// that is not parsed will have the wildcard "*" value (except r.endpoint).
// Potential underscores are stripped from _port and _protocol.
func parseRequest(name, zone string, multicluster, zonal bool) (r recordRequest, err error) {
	// 5 Possible cases:
	// 1. _port._protocol.service.namespace.pod|svc.zone
	// 2. (endpoint): endpoint.service.namespace.pod|svc.zone
	// 3. (service): service.namespace.pod|svc.zone
	// 4. (endpoint multicluster): endpoint.cluster.service.namespace.pod|svc.zone
	// 5. (zonal): topozone._zone.service.namespace.svc.zone

	base, _ := dnsutil.TrimZone(name, zone)
	// return NODATA for apex queries
	if base == "" || base == Svc || base == Pod {
		return r, nil
	}
	segs := dns.SplitDomainName(base)

	last := len(segs) - 1
	if last < 0 {
		return r, nil
	}
	r.podOrSvc = segs[last]
	if r.podOrSvc != Pod && r.podOrSvc != Svc {
		return r, errInvalidRequest
	}
	last--
	if last < 0 {
		return r, nil
	}

	r.namespace = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	r.service = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	// Because of ambiguity we check the labels left: 1: an endpoint. 2: port and protocol, endpoint and
	// clusterid, or a zone-scoped name.
	// Anything else is a query that is too long to answer and can safely be delegated to return an nxdomain.
	switch last {
	case 0: // endpoint only
		r.endpoint = segs[last]
	case 1: // port and protocol, endpoint and clusterid, or topology zone
		// Zonal names are not defined in multicluster zones, where the
		// two-labels-left shape belongs to the endpoint.clusterid grammar.
		if zonal && !multicluster && segs[last] == zoneLabel && r.podOrSvc == Svc {
			r.zone = segs[last-1]
		} else if !multicluster || strings.HasPrefix(segs[last], "_") || strings.HasPrefix(segs[last-1], "_") {
			r.protocol = stripUnderscore(segs[last])
			r.port = stripUnderscore(segs[last-1])
		} else {
			r.cluster = segs[last]
			r.endpoint = segs[last-1]
		}

	default: // too long
		return r, errInvalidRequest
	}

	return r, nil
}

// stripUnderscore removes a prefixed underscore from s.
func stripUnderscore(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] != '_' {
		return s
	}
	return s[1:]
}

// String returns a string representation of r, it just returns all fields concatenated with dots.
// This is mostly used in tests.
func (r recordRequest) String() string {
	s := r.port
	s += "." + r.protocol
	s += "." + r.endpoint
	s += "." + r.cluster
	s += "." + r.service
	s += "." + r.namespace
	s += "." + r.podOrSvc
	return s
}
