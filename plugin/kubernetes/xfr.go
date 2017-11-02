package kubernetes

import (
	"strings"

	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/client-go/pkg/api/v1"
)

// Serial implements the Transferer interface.
func (k *Kubernetes) Serial(state request.Request) uint32 { return uint32(k.APIConn.Modified()) }

// MinTTL implements the Transferer interface.
func (k *Kubernetes) MinTTL(state request.Request) uint32 { return 30 }

// Transfer implements the Transferer interface.
func (k *Kubernetes) Transfer(state request.Request) <-chan msg.Service {
	c := make(chan msg.Service)

	go k.transfer(c, state.Zone)

	return c
}

func (k *Kubernetes) transfer(c chan msg.Service, zone string) {

	defer close(c)

	zonePath := msg.Path(zone, "coredns")
	serviceList := k.APIConn.ServiceList()
	endpointsList := k.APIConn.EndpointsList()
	for _, svc := range serviceList {
		// Endpoint query or headless service
		if svc.Spec.ClusterIP == api.ClusterIPNone {
			for _, ep := range endpointsList {
				if ep.ObjectMeta.Name != svc.Name || ep.ObjectMeta.Namespace != svc.Namespace {
					continue
				}

				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						for _, p := range eps.Ports {

							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: k.ttl}
							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, endpointHostname(addr)}, "/")

							c <- s
						}
					}
				}
			}

			continue
		}

		// External service
		if svc.Spec.ExternalName != "" {
			s := msg.Service{Key: strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/"), Host: svc.Spec.ExternalName, TTL: k.ttl}
			if t, _ := s.HostType(); t == dns.TypeCNAME {
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")

				c <- s

				continue
			}
		}

		// ClusterIP service
		for _, p := range svc.Spec.Ports {

			s := msg.Service{Host: svc.Spec.ClusterIP, Port: int(p.Port), TTL: k.ttl}
			s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")

			c <- s
		}
	}
	return
}
