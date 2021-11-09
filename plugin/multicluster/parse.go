package multicluster

import (
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"strings"

	"github.com/miekg/dns"
)

type recordRequest struct {
	// The named port from the kubernetes DNS spec, this is the service part (think _https) from a well formed
	// SRV record.
	port string
	// The protocol is usually _udp or _tcp (if set), and comes from the protocol part of a well formed
	// SRV record.
	protocol string
	cluster  string
	endpoint string
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
func parseRequest(name, zone string) (r recordRequest, err error) {
	// 3 Possible cases:
	// 1. _port._protocol.service.namespace.pod|svc.zone
	// 2. (endpoint): endpoint.clusterid.service.namespace.pod|svc.zone
	// 3. (service): service.namespace.pod|svc.zone

	base, _ := dnsutil.TrimZone(name, zone)
	// return NODATA for apex queries
	if base == "" || base == Svc || base == Pod {
		return r, nil
	}
	segs := dns.SplitDomainName(base)

	r.port = "*"
	r.protocol = "*"

	// start at the right and fill out recordRequest with the bits we find, so we look for
	// pod|svc.namespace.service and then either
	// * endpoint.cluster
	// *_protocol._port

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

	// Because of ambiguity we check the labels left: 1: endpoint and cluster. 2: port and protocol.
	// Anything else is a query that is too long to answer and can safely be delegated to return an nxdomain.

	if last != 1 { // there must be exactly two labels remaining
		return r, errInvalidRequest
	}

	// TODO it doesn't support port and protocol wildcards
	// TODO unable to distinguish between endpoint+cluster vs protocol+port queries
	if strings.HasPrefix(segs[last], "_") { // if label starts with underscore, it must be port and protocol
		r.port = stripUnderscore(segs[last-1])
		r.protocol = stripUnderscore(segs[last])
	} else {
		r.endpoint = stripUnderscore(segs[last-1])
		r.cluster = stripUnderscore(segs[last])
	}

	return r, nil
}

// stripUnderscore removes a prefixed underscore from s.
func stripUnderscore(s string) string {
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
