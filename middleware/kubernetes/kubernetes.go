// Package kubernetes provides the kubernetes backend.
package kubernetes

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/etcd/msg"
	"github.com/miekg/coredns/middleware/kubernetes/nametemplate"
	"github.com/miekg/coredns/middleware/pkg/dnsutil"
	dnsstrings "github.com/miekg/coredns/middleware/pkg/strings"
	"github.com/miekg/coredns/middleware/proxy"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	unversionedapi "k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/1.5/tools/clientcmd/api"
)

// Kubernetes implements a middleware that connects to a Kubernetes cluster.
type Kubernetes struct {
	Next          middleware.Handler
	Zones         []string
	primaryZone   int
	Proxy         proxy.Proxy // Proxy for looking up names during the resolution process
	APIEndpoint   string
	APICertAuth   string
	APIClientCert string
	APIClientKey  string
	APIConn       *dnsController
	ResyncPeriod  time.Duration
	NameTemplate  *nametemplate.Template
	Namespaces    []string
	LabelSelector *unversionedapi.LabelSelector
	Selector      *labels.Selector
}

var errNoItems = errors.New("no items found")
var errNsNotExposed = errors.New("namespace is not exposed")

// Services implements the ServiceBackend interface.
func (k *Kubernetes) Services(state request.Request, exact bool, opt middleware.Options) ([]msg.Service, []msg.Service, error) {
	s, e := k.Records(state.Name(), exact)
	return s, nil, e // Haven't implemented debug queries yet.
}

// PrimaryZone will return the first non-reverse zone being handled by this middleware
func (k *Kubernetes) PrimaryZone() string {
	return k.Zones[k.primaryZone]
}

// Reverse implements the ServiceBackend interface.
func (k *Kubernetes) Reverse(state request.Request, exact bool, opt middleware.Options) ([]msg.Service, []msg.Service, error) {
	ip := dnsutil.ExtractAddressFromReverse(state.Name())
	if ip == "" {
		return nil, nil, nil
	}

	records := k.getServiceRecordForIP(ip, state.Name())
	return records, nil, nil
}

// Lookup implements the ServiceBackend interface.
func (k *Kubernetes) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return k.Proxy.Lookup(state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (k *Kubernetes) IsNameError(err error) bool {
	return err == errNoItems || err == errNsNotExposed
}

// Debug implements the ServiceBackend interface.
func (k *Kubernetes) Debug() string {
	return "debug"
}

func (k *Kubernetes) getClientConfig() (*rest.Config, error) {
	// For a custom api server or running outside a k8s cluster
	// set URL in env.KUBERNETES_MASTER or set endpoint in Corefile
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	clusterinfo := clientcmdapi.Cluster{}
	authinfo := clientcmdapi.AuthInfo{}
	if len(k.APIEndpoint) > 0 {
		clusterinfo.Server = k.APIEndpoint
	} else {
		cc, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return cc, err
	}
	if len(k.APICertAuth) > 0 {
		clusterinfo.CertificateAuthority = k.APICertAuth
	}
	if len(k.APIClientCert) > 0 {
		authinfo.ClientCertificate = k.APIClientCert
	}
	if len(k.APIClientKey) > 0 {
		authinfo.ClientKey = k.APIClientKey
	}
	overrides.ClusterInfo = clusterinfo
	overrides.AuthInfo = authinfo
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	return clientConfig.ClientConfig()
}

// InitKubeCache initializes a new Kubernetes cache.
func (k *Kubernetes) InitKubeCache() error {

	config, err := k.getClientConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("Failed to create kubernetes notification controller: %v", err)
	}

	if k.LabelSelector != nil {
		var selector labels.Selector
		selector, err = unversionedapi.LabelSelectorAsSelector(k.LabelSelector)
		k.Selector = &selector
		if err != nil {
			return fmt.Errorf("Unable to create Selector for LabelSelector '%s'.Error was: %s", k.LabelSelector, err)
		}
	}

	if k.LabelSelector == nil {
		log.Printf("[INFO] Kubernetes middleware configured without a label selector. No label-based filtering will be performed.")
	} else {
		log.Printf("[INFO] Kubernetes middleware configured with the label selector '%s'. Only kubernetes objects matching this label selector will be exposed.", unversionedapi.FormatLabelSelector(k.LabelSelector))
	}

	k.APIConn = newdnsController(kubeClient, k.ResyncPeriod, k.Selector)

	return err
}

// getPrefixesAndZoneForName returns the trailing zone string that matches
// the name, the leading SRV port, protocol, and reference id elements, if present, and the
// For example, if "coredns.local" is a zone configured for the
// Kubernetes middleware, then getZoneForName("_a._b.x.c.d.coredns.local")
// will return ("_a", "_b", "x", ["c", "d"], "coredns.local").
func (k *Kubernetes) getPrefixesAndZoneForName(name string) (string, string, []string, string) {
	var srvPort string
	var srvProtocol string
	var zone string
	var serviceSegments []string

	for _, z := range k.Zones {
		if dns.IsSubDomain(z, name) {
			zone = z

			serviceSegments = dns.SplitDomainName(name)
			serviceSegments = serviceSegments[:len(serviceSegments)-dns.CountLabel(zone)]
			break
		}
	}
	if strings.HasPrefix(serviceSegments[0], "_") {
		if strings.HasPrefix(serviceSegments[1], "_") {
			srvPort = serviceSegments[0]
			srvProtocol = serviceSegments[1]
			serviceSegments = serviceSegments[2:len(serviceSegments)]
		} else {
			srvProtocol = serviceSegments[0]
			serviceSegments = serviceSegments[1:len(serviceSegments)]
		}
	}

	return srvPort, srvProtocol, serviceSegments, zone
}

func parseServiceSegments(serviceSegments []string) (string, string, string, string) {
	serviceSegments, typeName := pullOffFromRight(serviceSegments)
	serviceSegments, namespace := pullOffFromRight(serviceSegments)
	serviceSegments, serviceName := pullOffFromRight(serviceSegments)
	serviceSegments, endpointName := pullOffFromRight(serviceSegments)

	if namespace == "" {
		err := errors.New("Parsing query string did not produce a namespace value. Assuming wildcard namespace.")
		log.Printf("[WARN] %v\n", err)
		namespace = "*"
	}

	if serviceName == "" {
		err := errors.New("Parsing query string did not produce a serviceName value. Assuming wildcard serviceName.")
		log.Printf("[WARN] %v\n", err)
		serviceName = "*"
	}
	return endpointName, serviceName, namespace, typeName
}

// Records looks up services in kubernetes. If exact is true, it will lookup
// just this name. This is used when find matches when completing SRV lookups
// for instance.
func (k *Kubernetes) Records(name string, exact bool) ([]msg.Service, error) {
	var records []msg.Service

	srvPort, srvProtocol, serviceSegments, zone := k.getPrefixesAndZoneForName(name)
	endpointName, serviceName, namespace, typeName := parseServiceSegments(serviceSegments)

	if typeName != "svc" {
		return nil, fmt.Errorf("%v not implemented", typeName)
	}

	serviceWildcard := symbolContainsWildcard(serviceName)
	nsWildcard := symbolContainsWildcard(namespace)
	epWildcard := symbolContainsWildcard(endpointName)
	srvPortWildcard := symbolContainsWildcard(srvPort)
	srvProtocolWildcard := symbolContainsWildcard(srvProtocol)

	// Abort early if the queried namespace is not in the Corefile namespace list.
	// Case where namespace is a wildcard is handled later
	if (!nsWildcard) && (len(k.Namespaces) > 0) && (!dnsstrings.StringInSlice(namespace, k.Namespaces)) {
		return nil, errNsNotExposed
	}

	servicesFound := false
	// Loop through all services in the cache
	for _, svc := range k.APIConn.ServiceList() {

		// continue to next service if namespace does not match
		if !symbolMatches(namespace, svc.Namespace, nsWildcard) {
			continue
		}
		// continue to next service if namespace is a wildcard and this service's namespace isnt in the Corefile namespace list.
		if nsWildcard && (len(k.Namespaces) > 0) && (!dnsstrings.StringInSlice(svc.Namespace, k.Namespaces)) {
			continue
		}

		// continue to next service if service name does not match
		if !symbolMatches(serviceName, svc.Name, serviceWildcard) {
			continue
		}

		domain := svc.Name + "." + svc.Namespace + ".svc." + zone
		if endpointName != "" {
			domain = endpointName + "." + domain
		}

		servicesFound = true
		if svc.Spec.ClusterIP != api.ClusterIPNone {
			// This is a cluster service with a Cluster IP. Create a record for each matching port
			for _, p := range svc.Spec.Ports {
				if !portAndProtocolMatch(srvPort, p.Name, srvPortWildcard, p.Port, srvProtocol, string(p.Protocol), srvProtocolWildcard) {
					continue
				}
				s := msg.Service{
					Key:         msg.Path(strings.ToLower("_"+p.Name+"._"+string(p.Protocol)+"."+domain), "coredns"),
					Host:        svc.Spec.ClusterIP,
					Port:        int(p.Port),
					TargetStrip: 0}
				records = append(records, s)

			}
		} else {
			// This is a "headless" service. Find all endpoints for this service in the Endpoints cache
			epList, _ := k.APIConn.epLister.List()
			for _, ep := range epList.Items {
				if !(ep.ObjectMeta.Name == svc.Name && ep.ObjectMeta.Namespace == svc.Namespace) {
					continue
				}
				for _, eps := range ep.Subsets {
					for _, port := range eps.Ports {
						if !portAndProtocolMatch(srvPort, port.Name, srvPortWildcard, port.Port, srvProtocol, string(port.Protocol), srvProtocolWildcard) {
							continue
						}
						for _, addr := range eps.Addresses {
							// Add all endpoints if no endpoint name was given (as if wildcard)
							// If a non wildcard endpoint name is given, only add that endpoint
							if endpointName != "" && symbolMatches(endpointName, getEndpointHostname(addr), epWildcard) {
								continue
							}
							epHostname := getEndpointHostname(addr)
							s := msg.Service{
								Key:         msg.Path(strings.ToLower("_"+port.Name+"._"+string(port.Protocol)+"."+epHostname+"."+domain), "coredns"),
								Host:        addr.IP,
								Port:        int(port.Port),
								TargetStrip: 0,
							}
							records = append(records, s)
						}

					}
				}
			}
		}

	}
	if len(records) == 0 && !servicesFound {
		// Did not find any matching services
		return nil, errNoItems
	}
	return records, nil

}

func getEndpointHostname(addr api.EndpointAddress) string {
	if addr.Hostname != "" {
		return addr.Hostname
	} else {
		return ipToEndpointId(addr.IP)
	}
}

// portAndProtocolMatch: Returns true if srvPort and srvProtocol match the portName/Num and portProtocol
func portAndProtocolMatch(srvPort string, portName string, srvPortWildcard bool, portNum int32, srvProtocol string, portProtocol string, srvProtocolWildcard bool) bool {
	// strip leading underscores if present
	if strings.HasPrefix(srvPort, "_") {
		srvPort = srvPort[1:len(srvPort)]
	}
	if strings.HasPrefix(srvProtocol, "_") {
		srvProtocol = srvProtocol[1:len(srvProtocol)]
	}
	return (srvPort == "" || symbolMatches(strings.ToLower(srvPort), strings.ToLower(portName), srvPortWildcard) || symbolMatches(srvPort, string(portNum), srvPortWildcard)) && (srvProtocol == "" || symbolMatches(strings.ToLower(srvProtocol), strings.ToLower(string(portProtocol)), srvProtocolWildcard))
}

func symbolMatches(queryString string, candidateString string, wildcard bool) bool {
	result := false
	switch {
	case !wildcard:
		result = (queryString == candidateString)
	case queryString == "*":
		result = true
	case queryString == "any":
		result = true
	}
	return result
}

// getServiceRecordForIP: Gets a service record with a cluster ip matching the ip argument
// If a service cluster ip does not match, it checks all endpoints
func (k *Kubernetes) getServiceRecordForIP(ip, name string) []msg.Service {
	// First check services with cluster ips
	svcList, err := k.APIConn.svcLister.List(labels.Everything())
	if err != nil {
		return nil
	}
	for _, service := range svcList {
		if !dnsstrings.StringInSlice(service.Namespace, k.Namespaces) {
			continue
		}
		if service.Spec.ClusterIP == ip {
			domain := service.Name + "." + service.Namespace + ".svc." + k.PrimaryZone()
			return []msg.Service{msg.Service{Host: domain}}
		}
	}
	// If no cluster ips match, search endpoints
	epList, err := k.APIConn.epLister.List()
	if err != nil {
		return nil
	}
	for _, ep := range epList.Items {
		if !dnsstrings.StringInSlice(ep.ObjectMeta.Namespace, k.Namespaces) {
			continue
		}
		for _, eps := range ep.Subsets {
			for _, addr := range eps.Addresses {
				if addr.IP == ip {
					domain := getEndpointHostname(addr) + "." + ep.ObjectMeta.Name + "." + ep.ObjectMeta.Namespace + ".svc." + k.PrimaryZone()
					return []msg.Service{msg.Service{Host: domain}}
				}
			}
		}
	}
	return nil
}

// ipToEndpointId converts an ip into a hostname segment
func ipToEndpointId(ip string) string {
	if strings.Contains(ip, ".") {
		return strings.Replace(ip, ".", "-", -1)
	}
	if strings.Contains(ip, ":") {
		return strings.ToLower(strings.Replace(ip, ":", "-", -1))
	}
	return ""
}

// symbolContainsWildcard checks whether symbol contains a wildcard value
func symbolContainsWildcard(symbol string) bool {
	return (strings.Contains(symbol, "*") || (symbol == "any"))
}

// pullOffFromRight pops the last element from a slice,
// It returns the rightmost element, and the slice with the last element removed
func pullOffFromRight(a []string) ([]string, string) {
	l := len(a)
	switch l {
	case 0:
		return a, ""
	case 1:
		return []string{}, a[0]
	}
	return a[:l-1], a[l-1]
}
