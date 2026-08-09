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
	// The servicename used in Kubernetes.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// maxSegs is the number of labels parseRequest keeps. The longest name it can answer
// for is _port._protocol.service.namespace.pod|svc, so one more slot than that is
// enough to recognise a name that is too long without counting the rest of it.
const maxSegs = 6

// splitReverse returns the labels of base in reverse order, so segs[0] is the label
// closest to the zone. The labels are stored in arr and sliced straight out of base,
// so the common case does not allocate. It reports false if base carries more labels
// than parseRequest can use.
//
// Slicing on '.' is only correct while a dot always separates two labels. An escaped
// dot is part of a label, and an empty label is not a label at all, so names
// containing either are handed to dns.SplitDomainName, which splits them the way the
// rest of CoreDNS does. Neither can name a real service, so the allocation it costs
// is not on any path that matters.
func splitReverse(base string, arr *[maxSegs]string) ([]string, bool) {
	// A leading dot is an empty first label, which the loop below would consume
	// without ever producing it.
	if base == "" || base[0] == '.' || strings.IndexByte(base, '\\') >= 0 {
		return splitReverseEscaped(base, arr)
	}

	segs := arr[:0]
	for end := len(base); end > 0; {
		idx := strings.LastIndexByte(base[:end], '.')
		label := base[idx+1 : end] // idx == -1 yields the first label
		end = idx
		if label == "" {
			return splitReverseEscaped(base, arr)
		}
		if len(segs) == cap(segs) {
			return nil, false
		}
		segs = append(segs, label)
	}
	return segs, true
}

func splitReverseEscaped(base string, arr *[maxSegs]string) ([]string, bool) {
	l := dns.SplitDomainName(base)
	if len(l) > len(arr) {
		return nil, false
	}
	segs := arr[:0]
	for i := len(l) - 1; i >= 0; i-- {
		segs = append(segs, l[i])
	}
	return segs, true
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

	var arr [maxSegs]string
	segs, ok := splitReverse(base, &arr)
	if !ok {
		return r, errInvalidRequest
	}

	n := len(segs)
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

	// Because of ambiguity we check the labels left: 1: an endpoint. 2: port and protocol or endpoint and clusterid.
	// Anything else is a query that is too long to answer and can safely be delegated to return an nxdomain.
	switch remaining := n - 3; remaining {
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
