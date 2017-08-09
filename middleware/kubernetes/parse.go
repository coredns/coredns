package kubernetes

import (
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type recordRequest struct {
	// The named port from the kubernetes DNS spec, this is the service part (think _https) from a well formed
	// SRV record.
	port string
	// The protocol is usually _udp or _tcp (if set), and comes from the protocol part of a well formed
	// SRV record.
	protocol  string
	endpoint  string
	service   string
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen. This value is true for
	// pods and false for services. If we ever need to extend this well use a typed value.
	podOrSvc   string
	zone       string
	federation string
}

// parse parses a qname destined for the kubernetes middleware, it tries to pick apart
// the qname so that we are left with the differnent parts; service, transport and namespace
// to name a few. The rcode returned indicated what we should do with the request.
func (k *Kubernetes) parse(state request.Request) (recordRequest, int) {
	if state.Zone == "" {
		return recordRequest{}, dns.RcodeServerFailure
	}

	r := recordRequest{}
	r.zone = state.Zone

	base, err := dnsutil.TrimZone(state.Name(), state.Zone)
	if err != nil {
		return r, dns.RcodeServerFailure
	}
	segs := dns.SplitDomainName(base)

	r.federation, segs = k.stripFederation(segs)

	// fluff.<namespace>.<pod|svc> should be left over.
	end := len(segs) - 1
	if segs[end] != Svc && segs[end] != Pod {
		return r, dns.RcodeNameError
	}

	r.podOrSvc = segs[end]

	// pod|svc seen, next namespace
	end--
	if end < 0 {
		// Wildcard namespace.
		r.namespace = "*"
		return r, dns.RcodeSuccess

	}

	return r, dns.RcodeSuccess
}

func (k *Kubernetes) parseRequest(lowerCasedName string, qtype uint16) (r recordRequest, err error) {
	// 3 Possible cases
	//   SRV Request: _port._protocol.service.namespace.[federation.]type.zone
	//   A Request (endpoint): endpoint.service.namespace.[federation.]type.zone
	//   A Request (service): service.namespace.[federation.]type.zone

	// separate zone from rest of lowerCasedName
	var segs []string
	for _, z := range k.Zones {
		if dns.IsSubDomain(z, lowerCasedName) {
			r.zone = z

			segs = dns.SplitDomainName(lowerCasedName)
			segs = segs[:len(segs)-dns.CountLabel(r.zone)]
			break
		}
	}
	if r.zone == "" {
		return r, errZoneNotFound
	}

	//defer func() {
	//fmt.Printf("rR %#v\n", r)
	//}()

	r.federation, segs = k.stripFederation(segs)

	if qtype == dns.TypeNS {
		return r, nil
	}

	if qtype == dns.TypeA && isDefaultNS(lowerCasedName, r) {
		return r, nil
	}

	offset := 0
	if qtype == dns.TypeSRV {
		// The kubernetes peer-finder expects queries with empty port and service to resolve
		// If neither is specified, treat it as a wildcard
		if len(segs) == 3 {
			r.port = "*"
			r.service = "*"
			offset = 0
		} else {
			if len(segs) != 5 {
				return r, errInvalidRequest
			}
			// This is a SRV style request, get first two elements as port and
			// protocol, stripping leading underscores if present.
			if segs[0][0] == '_' {
				r.port = segs[0][1:]
			} else {
				r.port = segs[0]
				if !wildcard(r.port) {
					return r, errInvalidRequest
				}
			}
			if segs[1][0] == '_' {
				r.protocol = segs[1][1:]
				if r.protocol != "tcp" && r.protocol != "udp" {
					return r, errInvalidRequest
				}
			} else {
				r.protocol = segs[1]
				if !wildcard(r.protocol) {
					return r, errInvalidRequest
				}
			}
			if r.port == "" || r.protocol == "" {
				return r, errInvalidRequest
			}
			offset = 2
		}
	}
	if (qtype == dns.TypeA || qtype == dns.TypeAAAA) && len(segs) == 4 {
		// This is an endpoint A/AAAA record request. Get first element as endpoint.
		r.endpoint = segs[0]
		offset = 1
	}

	if len(segs) == (offset + 3) {
		r.service = segs[offset]
		r.namespace = segs[offset+1]
		r.podOrSvc = segs[offset+2]

		return r, nil
	}

	return r, errInvalidRequest
}
