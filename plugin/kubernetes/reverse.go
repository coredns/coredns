package kubernetes

import (
	"context"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/request"
)

// Reverse implements the ServiceBackend interface.
func (k *Kubernetes) Reverse(ctx context.Context, state request.Request, exact bool, opt plugin.Options) ([]msg.Service, error) {
	ip := dnsutil.ExtractAddressFromReverse(state.Name())
	if ip == "" {
		_, e := k.Records(ctx, state, exact)
		return nil, e
	}

	records := k.serviceRecordForIP(ip, state.Name())
	if len(records) == 0 {
		return records, errNoItems
	}
	return records, nil
}

// serviceRecordForIP gets a service record with a cluster ip matching the ip argument
// If a service cluster ip does not match, it checks all endpoints
func (k *Kubernetes) serviceRecordForIP(ip, name string) []msg.Service {
	// First check services with cluster ips
	for _, service := range k.APIConn.SvcIndexReverse(ip) {
		if len(k.Namespaces) > 0 && !k.namespaceExposed(service.Namespace) {
			continue
		}
		domain := strings.Join([]string{service.Name, service.Namespace, Svc, k.primaryZone()}, ".")
		return []msg.Service{{Host: domain, TTL: k.ttl}}
	}
	// If no cluster ips match, search endpoints
	var svcs []msg.Service
	for _, ep := range k.APIConn.EpIndexReverse(ip) {
		if len(k.Namespaces) > 0 && !k.namespaceExposed(ep.Namespace) {
			continue
		}
		for _, eps := range ep.Subsets {
			for _, addr := range eps.Addresses {
				// The endpoint's Hostname will be set if this endpoint is supposed to generate a PTR.
				// So only return reverse records that match the IP AND have a non-empty hostname.
				// Kubernetes more or less keeps this to one canonical service/endpoint per IP, but in the odd event there
				// are multiple endpoints for the same IP with hostname set, return them all rather than selecting one
				// arbitrarily.
				if addr.IP == ip && addr.Hostname != "" {
					domain := strings.Join([]string{addr.Hostname, ep.Index, Svc, k.primaryZone()}, ".")
					svcs = append(svcs, msg.Service{Host: domain, TTL: k.ttl})
				}
			}
		}
	}
	return svcs
}
