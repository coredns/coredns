package kubernetes

import (
	"net"
	"strings"

	"github.com/coredns/coredns/middleware/etcd/msg"

	"github.com/miekg/dns"
	"k8s.io/client-go/1.5/pkg/api"
)

const nsName = "ns.dns."

var corednsRecord dns.A

// DefaultNSMsg returns an msg.Service representing an A record for
// ns.dns.[zone] -> dns service ip. This A record is needed to legitimize
// the SOA response in middleware.NS(), which is hardcoded at ns.dns.[zone].
func (k *Kubernetes) defaultNSMsg(r recordRequest) msg.Service {
	ns := k.nsAddress()
	s := msg.Service{
		Key:  msg.Path(strings.Join([]string{nsName, r.zone}, "."), "coredns"),
		Host: ns.A.String(),
	}
	return s
}

func isDefaultNS(name string, r recordRequest) bool {
	return strings.Index(name, nsName) == 0 && strings.Index(name, r.zone) == len(nsName)
}

func (k *Kubernetes) nsAddress() (addr *dns.A) {
	var (
		svcName      string
		svcNamespace string
		dnsIP        net.IP
	)

	// get local Pod IP
	localIP := k.interfaceAddrsFunc()
	// Find endpoint matching IP to get service and namespace
	endpointsList := k.APIConn.EndpointsList()

FindEndpoint:
	for _, ep := range endpointsList.Items {
		for _, eps := range ep.Subsets {
			for _, addr := range eps.Addresses {
				if localIP.Equal(net.ParseIP(addr.IP)) {

					svcNamespace = ep.ObjectMeta.Namespace
					svcName = ep.ObjectMeta.Name
					break FindEndpoint
				}
			}
		}
	}

	if len(svcName) == 0 {
		addr.Hdr.Name = nsName
		addr.A = localIP
		return addr
	}
	// Find service to get ClusterIP
	serviceList := k.APIConn.ServiceList()
FindService:
	for _, svc := range serviceList {
		if svcName == svc.Name && svcNamespace == svc.Namespace {
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

	addr.Hdr.Name = strings.Join([]string{svcName, svcNamespace, "svc."}, ".")
	addr.A = dnsIP

	return addr
}
