package kubernetes

import (
	"strings"

	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Those constants are used to distinguish between records in ExternalServices headless
// return values.
// They are always appendedn to key in a map which is
// either base service key eg. /com/example/namespace/service/endpoint or
// /com/example/namespace/service/_http/_tcp/port.protocol
// this will allow us to distinguish services in implementation of Transfer protocol
// see plugin/k8s_external/transfer.go
const (
	Endpoint     = "endpoint"
	PortProtocol = "port.protocol"
)

// External implements the ExternalFunc call from the external plugin.
// It returns any services matching in the services' ExternalIPs and if enabled, headless endpoints..
func (k *Kubernetes) External(state request.Request, headless bool) ([]msg.Service, int) {
	if state.QType() == dns.TypePTR {
		ip := dnsutil.ExtractAddressFromReverse(state.Name())
		if ip != "" {
			svcs, err := k.ExternalReverse(ip)
			if err != nil {
				return nil, dns.RcodeNameError
			}
			return svcs, dns.RcodeSuccess
		}
		// for invalid reverse names, fall through to determine proper nxdomain/nodata response
	}

	base, _ := dnsutil.TrimZone(state.Name(), state.Zone)

	segs := dns.SplitDomainName(base)
	last := len(segs) - 1
	if last < 0 {
		return nil, dns.RcodeServerFailure
	}
	// We are dealing with a fairly normal domain name here, but we still need to have the service,
	// namespace and if present, endpoint:
	// service.namespace.<base> or
	// endpoint.service.namespace.<base>
	var port, protocol, endpoint string
	namespace := segs[last]
	if !k.namespaceExposed(namespace) {
		return nil, dns.RcodeNameError
	}

	last--
	if last < 0 {
		return nil, dns.RcodeSuccess
	}

	service := segs[last]
	last--
	switch last {
	case 0:
		endpoint = stripUnderscore(segs[last])
		last--
	case 1:
		protocol = stripUnderscore(segs[last])
		port = stripUnderscore(segs[last-1])
		last -= 2
	}

	if last != -1 {
		// too long
		return nil, dns.RcodeNameError
	}

	var (
		endpointsList []*object.Endpoints
		serviceList   []*object.Service
	)

	idx := object.ServiceKey(service, namespace)
	serviceList = k.APIConn.SvcIndex(idx)

	services := []msg.Service{}
	zonePath := msg.Path(state.Zone, coredns)
	rcode := dns.RcodeNameError

	for _, svc := range serviceList {
		if namespace != svc.Namespace {
			continue
		}
		if service != svc.Name {
			continue
		}

		if headless && len(svc.ExternalIPs) == 0 && (svc.Headless() || endpoint != "") {
			if endpointsList == nil {
				endpointsList = k.APIConn.EpIndex(idx)
			}
			// Endpoint query or headless service
			for _, ep := range endpointsList {
				if object.EndpointsKey(svc.Name, svc.Namespace) != ep.Index {
					continue
				}

				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						if endpoint != "" && !match(endpoint, endpointHostname(addr, k.endpointNameMode)) {
							continue
						}

						for _, p := range eps.Ports {
							if !(matchPortAndProtocol(port, p.Name, protocol, p.Protocol)) {
								continue
							}
							rcode = dns.RcodeSuccess
							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: k.ttl}
							s.Key = strings.Join([]string{zonePath, svc.Namespace, svc.Name, endpointHostname(addr, k.endpointNameMode)}, "/")

							services = append(services, s)
						}
					}
				}
			}
			continue
		}
		for _, ip := range svc.ExternalIPs {
			for _, p := range svc.Ports {
				if !(matchPortAndProtocol(port, p.Name, protocol, string(p.Protocol))) {
					continue
				}
				rcode = dns.RcodeSuccess
				s := msg.Service{Host: ip, Port: int(p.Port), TTL: k.ttl}
				s.Key = strings.Join([]string{zonePath, svc.Namespace, svc.Name}, "/")

				services = append(services, s)
			}
		}
	}
	if state.QType() == dns.TypePTR {
		// if this was a PTR request, return empty service list, but retain rcode for proper nxdomain/nodata response
		return nil, rcode
	}
	return services, rcode
}

// ExternalAddress returns the external service address(es) for the CoreDNS service.
func (k *Kubernetes) ExternalAddress(state request.Request, headless bool) []dns.RR {
	// If CoreDNS is running inside the Kubernetes cluster: k.nsAddrs() will return the external IPs of the services
	// targeting the CoreDNS Pod.
	// If CoreDNS is running outside of the Kubernetes cluster: k.nsAddrs() will return the first non-loopback IP
	// address seen on the local system it is running on. This could be the wrong answer if coredns is using the *bind*
	// plugin to bind to a different IP address.
	return k.nsAddrs(true, headless, state.Zone)
}

// ExternalServices returns all services with external IPs and if enabled headless services
func (k *Kubernetes) ExternalServices(zone string, headless bool) (services []msg.Service, headlessServices map[string][]msg.Service) {
	zonePath := msg.Path(zone, coredns)
	headlessServices = make(map[string][]msg.Service)
	for _, svc := range k.APIConn.ServiceList() {
		// Endpoints and headless services
		if headless && len(svc.ExternalIPs) == 0 && svc.Headless() {
			idx := object.ServiceKey(svc.Name, svc.Namespace)
			endpointsList := k.APIConn.EpIndex(idx)

			for _, ep := range endpointsList {
				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						// we need to have some answers grouped together
						// 1. for endpoint requests eg. endpoint-0.service.example.com - will always have one endpoint
						// 2. for service requests eg. service.example.com - can have multiple endpoints
						// 3. for port.protocol requests eg. _http._tcp.service.example.com - can have multiple endpoints
						for _, p := range eps.Ports {
							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: k.ttl}
							baseSvc := strings.Join([]string{zonePath, svc.Namespace, svc.Name}, "/")
							s.Key = strings.Join([]string{baseSvc, endpointHostname(addr, k.endpointNameMode)}, "/")
							headlessServices[strings.Join([]string{baseSvc, Endpoint}, "/")] = append(headlessServices[strings.Join([]string{baseSvc, Endpoint}, "/")], s)

							// As per spec unnamed ports do not have a srv record
							// https://github.com/kubernetes/dns/blob/master/docs/specification.md#232---srv-records
							if p.Name == "" {
								continue
							}
							s.Host = msg.Domain(s.Key)
							s.Key = strings.Join(append([]string{zonePath, svc.Namespace, svc.Name}, strings.ToLower("_"+p.Protocol), strings.ToLower("_"+p.Name)), "/")
							headlessServices[strings.Join([]string{s.Key, PortProtocol}, "/")] = append(headlessServices[strings.Join([]string{s.Key, PortProtocol}, "/")], s)
						}
					}
				}
			}
			continue
		}
		for _, ip := range svc.ExternalIPs {
			for _, p := range svc.Ports {
				s := msg.Service{Host: ip, Port: int(p.Port), TTL: k.ttl}
				s.Key = strings.Join([]string{zonePath, svc.Namespace, svc.Name}, "/")
				services = append(services, s)
				s.Key = strings.Join(append([]string{zonePath, svc.Namespace, svc.Name}, strings.ToLower("_"+string(p.Protocol)), strings.ToLower("_"+p.Name)), "/")
				s.TargetStrip = 2
				services = append(services, s)
			}
		}
	}
	return services, headlessServices
}

// ExternalSerial returns the serial of the external zone
func (k *Kubernetes) ExternalSerial(string) uint32 {
	return uint32(k.APIConn.Modified(ModifiedExternal))
}

// ExternalReverse does a reverse lookup for the external IPs
func (k *Kubernetes) ExternalReverse(ip string) ([]msg.Service, error) {
	records := k.serviceRecordForExternalIP(ip)
	if len(records) == 0 {
		return records, errNoItems
	}
	return records, nil
}

func (k *Kubernetes) serviceRecordForExternalIP(ip string) []msg.Service {
	var svcs []msg.Service
	for _, service := range k.APIConn.SvcExtIndexReverse(ip) {
		if len(k.Namespaces) > 0 && !k.namespaceExposed(service.Namespace) {
			continue
		}
		domain := strings.Join([]string{service.Name, service.Namespace}, ".")
		svcs = append(svcs, msg.Service{Host: domain, TTL: k.ttl})
	}
	return svcs
}
