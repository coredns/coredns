package proxy

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// QueryExemplars selects query details to attach to request-duration samples.
// One instance is shared by the proxies in a forward stanza so sampling applies
// across its upstreams. It retains no query names.
type QueryExemplars struct {
	count atomic.Uint64
	zone  string
	every uint64
	types map[uint16]struct{}
}

// NewQueryExemplars samples every nth response matching zone and types.
// An empty types list matches all query types. The sampling interval must be positive.
func NewQueryExemplars(zone string, every uint64, types []uint16) (*QueryExemplars, error) {
	if zone == "" {
		return nil, fmt.Errorf("zone must not be empty")
	}
	zone = strings.ToLower(dns.Fqdn(zone))
	if _, ok := dns.IsDomainName(zone); !ok {
		return nil, fmt.Errorf("invalid zone %q", zone)
	}
	if every == 0 {
		return nil, fmt.Errorf("sampling interval must be positive")
	}
	q := &QueryExemplars{zone: zone, every: every, types: make(map[uint16]struct{}, len(types))}
	for _, t := range types {
		q.types[t] = struct{}{}
	}
	return q, nil
}

// SetQueryExemplars enables query exemplars, or disables them when q is nil.
// Call this before starting the proxy.
func (p *Proxy) SetQueryExemplars(q *QueryExemplars) { p.queryExemplars = q }

func (q *QueryExemplars) labels(state request.Request) prometheus.Labels {
	if q.every == 0 || state.Req == nil || len(state.Req.Question) == 0 {
		return nil
	}
	if len(q.types) != 0 {
		if _, ok := q.types[state.QType()]; !ok {
			return nil
		}
	}
	if !dns.IsSubDomain(q.zone, dns.Fqdn(state.Req.Question[0].Name)) {
		return nil
	}
	if q.every > 1 && q.count.Add(1)%q.every != 0 {
		return nil
	}
	// Only format query details for the selected samples.
	name := state.Name()
	qtype := state.Type()
	// ObserveWithExemplar panics on invalid labels. Long DNS names still need
	// their ordinary duration observation, without truncating the query detail.
	if !utf8.ValidString(name) || len("query_namequery_type")+utf8.RuneCountInString(name)+len(qtype) > prometheus.ExemplarMaxRunes {
		return nil
	}
	return prometheus.Labels{"query_name": name, "query_type": qtype}
}

func (p *Proxy) observeRequestDuration(state request.Request, rcode string, elapsed float64) {
	h := requestDuration.WithLabelValues(p.proxyName, p.addr, rcode)
	if p.queryExemplars != nil {
		if labels := p.queryExemplars.labels(state); labels != nil {
			// HistogramVec observers implement ExemplarObserver.
			h.(prometheus.ExemplarObserver).ObserveWithExemplar(elapsed, labels)
			return
		}
	}
	h.Observe(elapsed)
}
