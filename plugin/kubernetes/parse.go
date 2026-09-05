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

// maxSegs is the number of labels parseRequest can answer for without growing:
// _port._protocol.service.namespace.pod|svc. A zone-scoped name may be longer.
const maxSegs = 6

// splitReverse returns the labels of base in reverse order, so segs[0] is the label
// closest to the zone. The labels are sliced straight out of base into arr, so a name
// of the length parseRequest can answer for does not allocate. A zone-scoped name may
// carry more labels than that - the zone value may span labels - and grows off arr.
//
// Slicing on '.' is only correct while a dot always separates two labels. An escaped
// dot is part of a label, and an empty label is not a label at all, so names
// containing either are handed to dns.SplitDomainName, which splits them the way the
// rest of CoreDNS does. Neither can name a real service, so the allocation it costs
// is not on any path that matters.
func splitReverse(base string, arr *[maxSegs]string) []string {
	// A leading dot is an empty first label, which the loop below would consume
	// without ever producing it.
	if base == "" || base[0] == '.' || strings.IndexByte(base, '\\') >= 0 {
		return splitReverseEscaped(base)
	}

	segs := arr[:0]
	for end := len(base); end > 0; {
		idx := strings.LastIndexByte(base[:end], '.')
		label := base[idx+1 : end] // idx == -1 yields the first label
		end = idx
		if label == "" {
			return splitReverseEscaped(base)
		}
		segs = append(segs, label)
	}
	return segs
}

func splitReverseEscaped(base string) []string {
	l := dns.SplitDomainName(base)
	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
	return l
}

// joinReverse joins segs, which is in reverse label order, back into a name that
// reads left to right the way the query did.
func joinReverse(segs []string) string {
	if len(segs) == 1 {
		return segs[0]
	}
	size := len(segs) - 1
	for _, s := range segs {
		size += len(s)
	}
	var b strings.Builder
	b.Grow(size)
	for i := len(segs) - 1; i >= 0; i-- {
		if i != len(segs)-1 {
			b.WriteByte('.')
		}
		b.WriteString(segs[i])
	}
	return b.String()
}

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

	var arr [maxSegs]string
	segs := splitReverse(base, &arr)

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

	// Because of ambiguity we check the labels left: 1: an endpoint. 2: port and protocol
	// or endpoint and clusterid. 3 or more: a zone-scoped name (the zone value may span
	// labels). Anything else is a query that is too long to answer and can safely be
	// delegated to return an nxdomain.
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

	default: // zone-scoped name (topozone.pin|prefer._zone), or too long
		// Kubernetes zone label values may contain dots, so the zone is
		// every label left of the directive, joined. Not defined in
		// multicluster zones; everything this arm rejects keeps the stock
		// too-long NXDOMAIN, so behavior with the option off (or for
		// unknown directives) is byte-identical to today.
		if !zonal || multicluster || segs[3] != zoneLabel || r.podOrSvc != Svc {
			return r, errInvalidRequest
		}
		switch segs[4] {
		case directivePin:
		case directivePrefer:
			r.zonePrefer = true
		default:
			return r, errInvalidRequest
		}
		r.zone = joinReverse(segs[5:])
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
