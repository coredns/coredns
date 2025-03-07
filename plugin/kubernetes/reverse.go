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
	// Keep a discrete accounting of services with Hostnames set and those without
	// So that we don't return a mixed set of service records with and without Hostnames set
	var svcsHostname, svcsIPs []msg.Service
	for _, ep := range k.APIConn.EpIndexReverse(ip) {
		if len(k.Namespaces) > 0 && !k.namespaceExposed(ep.Namespace) {
			continue
		}
		for _, eps := range ep.Subsets {
			for _, addr := range eps.Addresses {
				var domain string
				if addr.IP == ip {
					if addr.Hostname != "" {
						// If the endpoint's Hostname is set, compose a compliant PTR record composed from that hostname
						// Kubernetes more or less keeps this to one canonical service/endpoint per IP, but in the odd event there
						// are multiple endpoints for the same IP with hostname set, return them all rather than selecting one
						// arbitrarily.
						domain = strings.Join([]string{addr.Hostname, ep.Index, Svc, k.primaryZone()}, ".")
						svcsHostname = append(svcsHostname, msg.Service{Host: domain, TTL: k.ttl})
					} else {
						// If the endpoint's Hostname is not set, generate a hostname from the IP address compose compliant PTR record
						domain = strings.Join([]string{endpointHostname(addr, k.endpointNameMode), ep.Index, Svc, k.primaryZone()}, ".")
						svcsIPs = append(svcsIPs, msg.Service{Host: domain, TTL: k.ttl})
					}
				}
			}
		}
	}
	// If our service list with Hostnames set is not empty, return it
	if len(svcsHostname) > 0 {
		return svcsHostname
	}
	// Otherwise return the service list with IPs
	return svcsIPs
}
