package kubernetes

import (
	"log"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
	api "k8s.io/client-go/pkg/api/v1"
)

// Serial implements the Transferer interface.
func (k *Kubernetes) Serial(state request.Request) uint32 { return uint32(k.APIConn.Modified()) }

// MinTTL implements the Transferer interface.
func (k *Kubernetes) MinTTL(state request.Request) uint32 { return 30 }

// Transfer implements the Transferer interface.
func (k *Kubernetes) Transfer(ctx context.Context, state request.Request) (int, error) {

	// Get all services.
	rrs := make(chan dns.RR)
	go k.transfer(rrs, state.Zone)

	records := []dns.RR{}
	for r := range rrs {
		records = append(records, r)
	}

	if len(records) == 0 {
		return dns.RcodeServerFailure, nil
	}

	ch := make(chan *dns.Envelope)
	defer close(ch)
	tr := new(dns.Transfer)
	go tr.Out(state.W, state.Req, ch)

	soa, err := plugin.SOA(k, state.Zone, state, plugin.Options{})
	if err != nil {
		return dns.RcodeServerFailure, nil
	}

	records = append(soa, records...)
	records = append(records, soa...)
	j, l := 0, 0
	log.Printf("[INFO] Outgoing transfer of %d records of zone %s to %s started", len(records), state.Zone, state.IP())
	for i, r := range records {
		l += dns.Len(r)
		if l > transferLength {
			ch <- &dns.Envelope{RR: records[j:i]}
			l = 0
			j = i
		}
	}
	if j < len(records) {
		ch <- &dns.Envelope{RR: records[j:]}
	}

	state.W.Hijack()
	// w.Close() // Client closes connection
	return dns.RcodeSuccess, nil
}

func (k *Kubernetes) transfer(c chan dns.RR, zone string) {

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

					// Default everything to SRV, for lack of ideas.
					c <- s.NewSRV(s.Key, 8080)

					s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, strings.ToLower("_" + string(p.Protocol)), strings.ToLower("_" + string(p.Name))}, "/")

					c <- s.NewSRV(s.Key, 8080)
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

							ip := net.ParseIP(s.Host)

							c <- s.NewA(s.Key, ip)

							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, strings.ToLower("_" + string(p.Protocol)), strings.ToLower("_" + string(p.Name))}, "/")

							c <- s.NewA(s.Key, ip)
						}
					}
				}
			}

		case api.ServiceTypeExternalName:

			s := msg.Service{Key: strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/"), Host: svc.Spec.ExternalName, TTL: k.ttl}
			if t, _ := s.HostType(); t == dns.TypeCNAME {
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")

				c <- s.NewCNAME(s.Key, svc.Name) // whatever...
			}
		}
	}
	return
}

const transferLength = 2000
