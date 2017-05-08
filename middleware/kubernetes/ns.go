package kubernetes

import (
	"net"

	"github.com/coredns/coredns/middleware/etcd/msg"

	"github.com/miekg/dns"
	"k8s.io/client-go/1.5/pkg/api"
)

const DefaultNSName = "ns.dns."

var corednsRecord dns.A

func (k *Kubernetes) recordsForNS(r recordRequest) ([]msg.Service, error) {
	ns := k.CoreDNSRecord()
	s := msg.Service{
		Host: ns.A.String(),
		Key:  msg.Path(ns.Hdr.Name+r.zone, "coredns")}
	return []msg.Service{s}, nil
}

// DefaultNSMsg returns an msg.Service representing an A record for
// ns.dns.[zone] -> dns service ip. This A record is needed to legitimize
// the SOA response in middleware.NS(), which is hardcoded at ns.dns.[zone].
func (k *Kubernetes) DefaultNSMsg(r recordRequest) msg.Service {
	ns := k.CoreDNSRecord()
	s := msg.Service{
		Key:  msg.Path(DefaultNSName+r.zone, "coredns"),
		Host: ns.A.String(),
	}
	return s
}

func IsDefaultNS(name string, r recordRequest) bool {
	return name == DefaultNSName+"."+r.zone
}

func (k *Kubernetes) CoreDNSRecord() dns.A {
	var localIP net.IP

	dnsName := corednsRecord.Hdr.Name
	dnsIP := corednsRecord.A

	if dnsName == "" {
		// get local Pod IP
		addrs, _ := net.InterfaceAddrs()
		for _, addr := range addrs {
			ip, _, _ := net.ParseCIDR(addr.String())
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			localIP = ip
			break
		}
		// Find endpoint matching IP to get service and namespace
		endpointsList, err := k.APIConn.epLister.List()
		if err != nil {
			return corednsRecord
		}
	FindEndpoint:
		for _, ep := range endpointsList.Items {
			for _, eps := range ep.Subsets {
				for _, addr := range eps.Addresses {
					if localIP.Equal(net.ParseIP(addr.IP)) {
						dnsName = ep.ObjectMeta.Name + "." + ep.ObjectMeta.Namespace + ".svc."
						break FindEndpoint
					}
				}
			}
		}
		if dnsName == "" {
			corednsRecord.Hdr.Name = DefaultNSName
			corednsRecord.A = localIP
			return corednsRecord
		}
	}
	if dnsIP == nil {
		// Find service to get ClusterIP
		serviceList := k.APIConn.ServiceList()
	FindService:
		for _, svc := range serviceList {
			if dnsName == svc.Name+"."+svc.Namespace+".svc." {
				if svc.Spec.ClusterIP == api.ClusterIPNone {
					dnsIP = localIP
				} else {
					dnsIP = net.ParseIP(svc.Spec.ClusterIP)
				}
				break FindService
			}
		}
		if dnsIP == nil {
			dnsIP = localIP
		}
	}
	corednsRecord.Hdr.Name = dnsName
	corednsRecord.A = dnsIP
	return corednsRecord
}
