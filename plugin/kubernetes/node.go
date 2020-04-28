package kubernetes

import (
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"strings"
)

func (k *Kubernetes) Node(state request.Request) ([]msg.Service, int) {
	node := state.Name()[:len(state.Name())-1]
	nodeList := k.APIConn.NodeIndex(node)
	services := []msg.Service{}
	zonePath := msg.Path(state.Zone, coredns)
	rcode := dns.RcodeNameError

	for _, n := range nodeList {
		if node != n.Name {
			continue
		}

		for _, ip := range n.ExternalIPS {
			rcode = dns.RcodeSuccess
			s := msg.Service{Host: ip, TTL: k.ttl}
			s.Key = strings.Join([]string{zonePath, strings.ReplaceAll(n.Name, ".", "/")}, "/")
			services = append(services, s)
		}

		for _, ip := range n.InternalIPS {
			rcode = dns.RcodeSuccess
			s := msg.Service{Host: ip, TTL: k.ttl}
			s.Key = strings.Join([]string{zonePath, strings.ReplaceAll(n.Name, ".", "/")}, "/")
			services = append(services, s)
		}
	}
	return services, rcode
}

// ExternalAddress returns the external service address(es) for the CoreDNS service.
func (k *Kubernetes) NodeAddress(state request.Request) []dns.RR {
	// If CoreDNS is running inside the Kubernetes cluster: k.nsAddrs() will return the external IPs of the services
	// targeting the CoreDNS Pod.
	// If CoreDNS is running outside of the Kubernetes cluster: k.nsAddrs() will return the first non-loopback IP
	// address seen on the local system it is running on. This could be the wrong answer if coredns is using the *bind*
	// plugin to bind to a different IP address.
	return []dns.RR{}
}
