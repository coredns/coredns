package kubernetes

import (
	"context"
	"errors"
	"math"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
)

// Transfer implements transfer.Transferer
func (k *Kubernetes) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	ch := make(chan []dns.RR, 2)

	// look for exact zone match
	var found bool
	for _, kz := range k.Zones {
		if zone == kz {
			found = true
			break
		}
	}
	if !found {
		close(ch)
		return nil, transfer.ErrNotAuthoritative
	}

	// reverse zone transfers are not implemented
	if rev := (plugin.Zones{"in-addr.arpa.", "ip6.arpa."}).Matches(zone); rev != "" {
		close(ch)
		return nil, transfer.ErrNotAuthoritative
	}

	// SOA record
	soas, err := plugin.SOA(context.TODO(), k, zone, request.Request{}, plugin.Options{})
	if err != nil {
		close(ch)
		return nil, errors.New("could not create soa")
	}
	soa, ok := soas[0].(*dns.SOA)
	if !ok {
		close(ch)
		return nil, errors.New("invalid soa")
	}
	ch <- []dns.RR{soa}

	if serial >= soa.Serial && serial != 0 {
		// Zone is up to date. Just return the SOA
		close(ch)
		return ch, nil
	}

	go func() {
		defer close(ch)
		// Add NS and glue records
		seen := map[string]bool{}
		header := dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Ttl: k.ttl, Class: dns.ClassINET}
		nsAddrs := k.nsAddrs(false, zone)
		for _, nsAddr := range nsAddrs {
			if !seen[nsAddr.Header().Name] {
				ns := &dns.NS{Hdr: header, Ns: nsAddr.Header().Name}
				ch <- []dns.RR{ns}
				seen[nsAddr.Header().Name] = true
			}
			ch <- []dns.RR{nsAddr}
		}

		// Add All Service/Endpoint records
		k.transfer(ch, zone)
	}()

	return ch, nil
}

func (k *Kubernetes) transfer(c chan []dns.RR, zone string) {
	zonePath := msg.Path(zone, "coredns")
	serviceList := k.APIConn.ServiceList()
	for _, svc := range serviceList {
		if !k.namespaceExposed(svc.Namespace) {
			continue
		}
		svcBase := []string{zonePath, Svc, svc.Namespace, svc.Name}
		switch svc.Type {
		case api.ServiceTypeClusterIP, api.ServiceTypeNodePort, api.ServiceTypeLoadBalancer:
			clusterIP := net.ParseIP(svc.ClusterIP)
			if clusterIP != nil {
				s := msg.Service{Host: svc.ClusterIP, TTL: k.ttl}
				s.Key = strings.Join(svcBase, "/")

				// Change host from IP to Name for SRV records
				host := emitAddressRecord(c, s)

				for _, p := range svc.Ports {
					s := msg.Service{Host: host, Port: int(p.Port), TTL: k.ttl}
					s.Key = strings.Join(svcBase, "/")

					// Need to generate this to handle use cases for peer-finder
					// ref: https://github.com/coredns/coredns/pull/823
					c <- []dns.RR{s.NewSRV(msg.Domain(s.Key), 100)}

					// As per spec unnamed ports do not have a srv record
					// https://github.com/kubernetes/dns/blob/master/docs/specification.md#232---srv-records
					if p.Name == "" {
						continue
					}

					s.Key = strings.Join(append(svcBase, strings.ToLower("_"+string(p.Protocol)), strings.ToLower("_"+string(p.Name))), "/")

					c <- []dns.RR{s.NewSRV(msg.Domain(s.Key), 100)}
				}

				//  Skip endpoint discovery if clusterIP is defined
				continue
			}

			endpointsList := k.APIConn.EpIndex(svc.Name + "." + svc.Namespace)

			for _, ep := range endpointsList {
				if ep.Name != svc.Name || ep.Namespace != svc.Namespace {
					continue
				}

				for _, eps := range ep.Subsets {
					srvWeight := calcSRVWeight(len(eps.Addresses))
					for _, addr := range eps.Addresses {
						s := msg.Service{Host: addr.IP, TTL: k.ttl}
						s.Key = strings.Join(svcBase, "/")
						// We don't need to change the msg.Service host from IP to Name yet
						// so disregard the return value here
						emitAddressRecord(c, s)

						s.Key = strings.Join(append(svcBase, endpointHostname(addr, k.endpointNameMode)), "/")
						// Change host from IP to Name for SRV records
						host := emitAddressRecord(c, s)
						s.Host = host

						for _, p := range eps.Ports {
							// As per spec unnamed ports do not have a srv record
							// https://github.com/kubernetes/dns/blob/master/docs/specification.md#232---srv-records
							if p.Name == "" {
								continue
							}

							s.Port = int(p.Port)

							s.Key = strings.Join(append(svcBase, strings.ToLower("_"+string(p.Protocol)), strings.ToLower("_"+string(p.Name))), "/")
							c <- []dns.RR{s.NewSRV(msg.Domain(s.Key), srvWeight)}
						}
					}
				}
			}

		case api.ServiceTypeExternalName:

			s := msg.Service{Key: strings.Join(svcBase, "/"), Host: svc.ExternalName, TTL: k.ttl}
			if t, _ := s.HostType(); t == dns.TypeCNAME {
				c <- []dns.RR{s.NewCNAME(msg.Domain(s.Key), s.Host)}
			}
		}
	}
	return
}

// emitAddressRecord generates a new A or AAAA record based on the msg.Service and writes it to
// a channel.
// emitAddressRecord returns the host name from the generated record.
func emitAddressRecord(c chan []dns.RR, message msg.Service) string {
	ip := net.ParseIP(message.Host)
	var host string
	dnsType, _ := message.HostType()
	switch dnsType {
	case dns.TypeA:
		arec := message.NewA(msg.Domain(message.Key), ip)
		host = arec.Hdr.Name
		c <- []dns.RR{arec}
	case dns.TypeAAAA:
		arec := message.NewAAAA(msg.Domain(message.Key), ip)
		host = arec.Hdr.Name
		c <- []dns.RR{arec}
	}

	return host
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

	return uint16(math.Floor((100.0 / float64(w[0])) * 100))
}
