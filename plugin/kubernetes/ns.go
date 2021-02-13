package kubernetes

import (
	"net"
	"strings"

	"github.com/miekg/dns"
)

func isDefaultNS(name, zone string) bool {
	return strings.Index(name, defaultNSName) == 0 && strings.Index(name, zone) == len(defaultNSName)
}

// nsAddrs returns the A or AAAA records for the CoreDNS service in the cluster. If the service cannot be found,
// it returns a record for the local address of the machine we're running on.
func (k *Kubernetes) nsAddrs(external bool, zone string) []dns.RR {
	var (
		svcNames []string
		svcIPs   []net.IP
	)

	// Find the CoreDNS Endpoints
	for _, localIP := range k.localIPs {
		endpoints := k.APIConn.EpIndexReverse(localIP.String())

		// Collect IPs for all Services of the Endpoints
		for _, endpoint := range endpoints {
			svcs := k.APIConn.SvcIndex(endpoint.Index)
			for _, svc := range svcs {
				if external {
					svcName := strings.Join([]string{svc.Name, svc.Namespace, zone}, ".")
					for _, exIP := range svc.ExternalIPs {
						svcNames = append(svcNames, svcName)
						svcIPs = append(svcIPs, net.ParseIP(exIP))
					}
					continue
				}
				svcName := strings.Join([]string{svc.Name, svc.Namespace, Svc, zone}, ".")
				if svc.Headless() {
					// For a headless service, use the endpoints IPs
					for _, s := range endpoint.Subsets {
						for _, a := range s.Addresses {
							svcNames = append(svcNames, endpointHostname(a, k.endpointNameMode)+"."+svcName)
							svcIPs = append(svcIPs, net.ParseIP(a.IP))
						}
					}
				} else {
					for _, clusterIP := range svc.ClusterIPs {
						svcNames = append(svcNames, svcName)
						svcIPs = append(svcIPs, net.ParseIP(clusterIP))
					}
				}
			}
		}
	}

	// If no local IPs matched any endpoints, use the localIPs directly
	if len(svcIPs) == 0 {
		svcIPs = make([]net.IP, len(k.localIPs))
		svcNames = make([]string, len(k.localIPs))
		for i, localIP := range k.localIPs {
			svcNames[i] = defaultNSName + zone
			svcIPs[i] = localIP
		}
	}

	// Create an RR slice of collected IPs
	rrs := make([]dns.RR, len(svcIPs))
	for i, ip := range svcIPs {
		if ip.To4() == nil {
			rr := new(dns.AAAA)
			rr.Hdr.Class = dns.ClassINET
			rr.Hdr.Rrtype = dns.TypeAAAA
			rr.Hdr.Name = svcNames[i]
			rr.AAAA = ip
			rrs[i] = rr
			continue
		}
		rr := new(dns.A)
		rr.Hdr.Class = dns.ClassINET
		rr.Hdr.Rrtype = dns.TypeA
		rr.Hdr.Name = svcNames[i]
		rr.A = ip
		rrs[i] = rr
	}

	return rrs
}

const defaultNSName = "ns.dns."
