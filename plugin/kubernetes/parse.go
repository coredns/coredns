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
	// The topology zone from a zone-scoped name
	// (zone.pin._zone.service.namespace.svc.zone); only set when the zonal
	// option is enabled.
	zone string
	// zonePrefer is set for the prefer directive: a zone holding no
	// endpoints falls back to all endpoints. The zero value is the pin
	// directive, which answers NODATA instead.
	zonePrefer bool
	// The servicename used in Kubernetes.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// zoneLabel anchors a zone-scoped name:
// topozone.DIRECTIVE._zone.service.namespace.svc.zone. It sits three labels
// left of the service — a shape that has always been "query too long"
// (NXDOMAIN) — and the underscore keeps it out of every hostname-shaped
// grammar, so nothing served or servable collides with it. The directive
// label selects the semantics; bare words are safe there because the
// subtree is only reachable through the anchor.
const zoneLabel = "_zone"

// Zone-scoped name directives.
const (
	directivePin    = "pin"    // zone-local endpoints, NODATA if none
	directivePrefer = "prefer" // zone-local endpoints, all endpoints if none
)

// parseRequest parses the qname to find all the elements we need for querying k8s. Anything
// that is not parsed will have the wildcard "*" value (except r.endpoint).
// Potential underscores are stripped from _port and _protocol.
func parseRequest(name, zone string, multicluster, zonal bool) (r recordRequest, err error) {
	// 5 Possible cases:
	// 1. _port._protocol.service.namespace.pod|svc.zone
	// 2. (endpoint): endpoint.service.namespace.pod|svc.zone
	// 3. (service): service.namespace.pod|svc.zone
	// 4. (endpoint multicluster): endpoint.cluster.service.namespace.pod|svc.zone
	// 5. (zonal): topozone.pin|prefer._zone.service.namespace.svc.zone

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

	// Because of ambiguity we check the labels left: 1: an endpoint. 2: port and protocol or endpoint
	// and clusterid. 3: a zone-scoped name.
	// Anything else is a query that is too long to answer and can safely be delegated to return an nxdomain.
	switch last {
	case 0: // endpoint only
		r.endpoint = segs[last]
	case 1: // service and port or endpoint and clusterid
		if !multicluster || strings.HasPrefix(segs[last], "_") || strings.HasPrefix(segs[last-1], "_") {
			r.protocol = stripUnderscore(segs[last])
			r.port = stripUnderscore(segs[last-1])
		} else {
			r.cluster = segs[last]
			r.endpoint = segs[last-1]
		}

	case 2: // zone-scoped name: topozone.pin|prefer._zone
		// Not defined in multicluster zones; everything this arm rejects
		// keeps the stock too-long NXDOMAIN, so behavior with the option
		// off (or for unknown directives) is byte-identical to today.
		if !zonal || multicluster || segs[last] != zoneLabel || r.podOrSvc != Svc {
			return r, errInvalidRequest
		}
		switch segs[last-1] {
		case directivePin:
		case directivePrefer:
			r.zonePrefer = true
		default:
			return r, errInvalidRequest
		}
		r.zone = segs[last-2]

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
