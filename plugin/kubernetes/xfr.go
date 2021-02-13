package kubernetes

import (
	"context"
	"math"
	"net"
	"sort"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

// Transfer implements the transfer.Transfer interface.
func (k *Kubernetes) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	// state is not used here, hence the empty request.Request{]
	soa, err := plugin.SOA(context.TODO(), k, zone, request.Request{}, plugin.Options{})
	if err != nil {
		return nil, transfer.ErrNotAuthoritative
	}

	ch := make(chan []dns.RR)

	zonePath := msg.Path(zone, "coredns")
	serviceList := k.APIConn.ServiceList()

	go func() {
		// ixfr fallback
		if serial != 0 && soa[0].(*dns.SOA).Serial == serial {
			ch <- soa
			close(ch)
			return
		}
		ch <- soa

		sort.Slice(serviceList, func(i, j int) bool {
			return serviceList[i].Name < serviceList[j].Name
		})

		for _, svc := range serviceList {
			if !k.namespaceExposed(svc.Namespace) {
				continue
			}
			svcBase := []string{zonePath, Svc, svc.Namespace, svc.Name}
			switch svc.Type {

			case api.ServiceTypeClusterIP, api.ServiceTypeNodePort, api.ServiceTypeLoadBalancer:
				clusterIP := net.ParseIP(svc.ClusterIPs[0])
				if clusterIP != nil {
					var host string
					for _, ip := range svc.ClusterIPs {
						s := msg.Service{Host: ip, TTL: k.ttl}
						s.Key = strings.Join(svcBase, "/")

						// Change host from IP to Name for SRV records
						host = emitAddressRecord(ch, s)
					}

					for _, p := range svc.Ports {
						s := msg.Service{Host: host, Port: int(p.Port), TTL: k.ttl}
						s.Key = strings.Join(svcBase, "/")

						// Need to generate this to handle use cases for peer-finder
						// ref: https://github.com/coredns/coredns/pull/823
						ch <- []dns.RR{s.NewSRV(msg.Domain(s.Key), 100)}

						// As per spec unnamed ports do not have a srv record
						// https://github.com/kubernetes/dns/blob/master/docs/specification.md#232---srv-records
						if p.Name == "" {
							continue
						}

						s.Key = strings.Join(append(svcBase, strings.ToLower("_"+string(p.Protocol)), strings.ToLower("_"+string(p.Name))), "/")

						ch <- []dns.RR{s.NewSRV(msg.Domain(s.Key), 100)}
					}

					//  Skip endpoint discovery if clusterIP is defined
					continue
				}

				endpointsList := k.APIConn.EpIndex(svc.Name + "." + svc.Namespace)

				for _, ep := range endpointsList {
					for _, eps := range ep.Subsets {
						srvWeight := calcSRVWeight(len(eps.Addresses))
						for _, addr := range eps.Addresses {
							s := msg.Service{Host: addr.IP, TTL: k.ttl}
							s.Key = strings.Join(svcBase, "/")
							// We don't need to change the msg.Service host from IP to Name yet
							// so disregard the return value here
							emitAddressRecord(ch, s)

							s.Key = strings.Join(append(svcBase, endpointHostname(addr, k.endpointNameMode)), "/")
							// Change host from IP to Name for SRV records
							host := emitAddressRecord(ch, s)
							s.Host = host

							for _, p := range eps.Ports {
								// As per spec unnamed ports do not have a srv record
								// https://github.com/kubernetes/dns/blob/master/docs/specification.md#232---srv-records
								if p.Name == "" {
									continue
								}

								s.Port = int(p.Port)

								s.Key = strings.Join(append(svcBase, strings.ToLower("_"+string(p.Protocol)), strings.ToLower("_"+string(p.Name))), "/")
								ch <- []dns.RR{s.NewSRV(msg.Domain(s.Key), srvWeight)}
							}
						}
					}
				}

			case api.ServiceTypeExternalName:

				s := msg.Service{Key: strings.Join(svcBase, "/"), Host: svc.ExternalName, TTL: k.ttl}
				if t, _ := s.HostType(); t == dns.TypeCNAME {
					ch <- []dns.RR{s.NewCNAME(msg.Domain(s.Key), s.Host)}
				}
			}
		}
		ch <- soa
		close(ch)
	}()
	return ch, nil
}

// emitAddressRecord generates a new A or AAAA record based on the msg.Service and writes it to a channel.
// emitAddressRecord returns the host name from the generated record.
func emitAddressRecord(c chan<- []dns.RR, s msg.Service) string {
	ip := net.ParseIP(s.Host)
	dnsType, _ := s.HostType()
	switch dnsType {
	case dns.TypeA:
		r := s.NewA(msg.Domain(s.Key), ip)
		c <- []dns.RR{r}
		return r.Hdr.Name
	case dns.TypeAAAA:
		r := s.NewAAAA(msg.Domain(s.Key), ip)
		c <- []dns.RR{r}
		return r.Hdr.Name
	}

	return ""
}

// calcSrvWeight borrows the logic implemented in plugin.SRV for dynamically
// calculating the srv weight and priority
func calcSRVWeight(numservices int) uint16 {
	var services []msg.Service

	for i := 0; i < numservices; i++ {
		services = append(services, msg.Service{})
	}

	w := make(map[int]int)
	for _, serv := range services {
		weight := 100
		if serv.Weight != 0 {
			weight = serv.Weight
		}
		if _, ok := w[serv.Priority]; !ok {
			w[serv.Priority] = weight
			continue
		}
		w[serv.Priority] += weight
	}
	weight := uint16(math.Floor((100.0 / float64(w[0])) * 100))
	// weight should be at least 1
	if weight == 0 {
		weight = 1
	}

	return weight
}
