package kubernetes

import (
	"strings"

	"github.com/coredns/coredns/plugin/pkg/dnsutil"
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
	// The servicename used in Kubernetes.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// parseRequest parses the qname to find all the elements we need for querying k8s. Anything
// that is not parsed will have the wildcard "*" value (except r.endpoint).
// Potential underscores are stripped from _port and _protocol.
func parseRequest(name, zone string, multicluster bool) (r recordRequest, err error) {
	// 4 Possible cases:
	// 1. _port._protocol.service.namespace.pod|svc.zone
	// 2. (endpoint): endpoint.service.namespace.pod|svc.zone
	// 3. (service): service.namespace.pod|svc.zone
	// 4. (endpoint multicluster): endpoint.cluster.service.namespace.pod|svc.zone

	base, _ := dnsutil.TrimZone(name, zone)
	// return NODATA for apex queries
	if base == "" || base == Svc || base == Pod {
		return r, nil
	}

	var segs [6]string
	n := 0
	end := len(base)
	for end > 0 {
		idx := strings.LastIndexByte(base[:end], '.')
		var label string
		if idx == -1 {
			label = base[:end]
			end = 0
		} else {
			label = base[idx+1 : end]
			end = idx
		}
		if label == "" {
			continue
		}
		if n >= 6 {
			return r, errInvalidRequest
		}
		segs[n] = label
		n++
	}

	if n < 1 {
		return r, nil
	}
	r.podOrSvc = segs[0]
	if r.podOrSvc != Pod && r.podOrSvc != Svc {
		return r, errInvalidRequest
	}
	if n < 2 {
		return r, nil
	}

	r.namespace = segs[1]
	if n < 3 {
		return r, nil
	}

	r.service = segs[2]
	if n < 4 {
		return r, nil
	}

	remaining := n - 3
	switch remaining {
	case 1: // endpoint only
		r.endpoint = segs[3]
	case 2: // service and port or endpoint and clusterid
		if !multicluster || strings.HasPrefix(segs[3], "_") || strings.HasPrefix(segs[4], "_") {
			r.protocol = stripUnderscore(segs[3])
			r.port = stripUnderscore(segs[4])
		} else {
			r.cluster = segs[3]
			r.endpoint = segs[4]
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
