package kubernetes

import (
	"net"
	"strings"

	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
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
	for _, svc := range serviceList {
		switch svc.Spec.Type {
		case api.ServiceTypeClusterIP, api.ServiceTypeNodePort, api.ServiceTypeLoadBalancer:

			if net.ParseIP(svc.Spec.ClusterIP) != nil {
				for _, p := range svc.Spec.Ports {

					s := msg.Service{Host: svc.Spec.ClusterIP, Port: int(p.Port), TTL: k.ttl}
					s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")

					c <- s

					s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, strings.ToLower("_" + string(p.Protocol)), strings.ToLower("_" + string(p.Name))}, "/")

					c <- s
				}

				//  Skip endpoint discovery if clusterIP is defined
				continue
			}

			endpointsList := k.APIConn.EpIndex(svc.Name + "." + svc.Namespace)

			for _, ep := range endpointsList {
				if ep.ObjectMeta.Name != svc.Name || ep.ObjectMeta.Namespace != svc.Namespace {
					continue
				}

				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						for _, p := range eps.Ports {

							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: k.ttl}
							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, endpointHostname(addr, k.endpointNameMode)}, "/")

							c <- s

							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, strings.ToLower("_" + string(p.Protocol)), strings.ToLower("_" + string(p.Name))}, "/")

							c <- s
						}
					}
				}
			}

		case api.ServiceTypeExternalName:

			s := msg.Service{Key: strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/"), Host: svc.Spec.ExternalName, TTL: k.ttl}
			if t, _ := s.HostType(); t == dns.TypeCNAME {
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")

				c <- s
			}
		}

	}
	return
}
