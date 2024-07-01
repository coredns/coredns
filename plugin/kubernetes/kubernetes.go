// Package kubernetes provides the kubernetes backend.
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/coredns/coredns/coremain"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Kubernetes implements a plugin that connects to a Kubernetes cluster.
type Kubernetes struct {
	Next             plugin.Handler
	Zones            []string
	Upstream         Upstreamer
	APIServerList    []string
	APICertAuth      string
	APIClientCert    string
	APIClientKey     string
	ClientConfig     clientcmd.ClientConfig
	APIConn          dnsController
	Namespaces       map[string]struct{}
	podMode          string
	endpointNameMode bool
	Fall             fall.F
	ttl              uint32
	opts             dnsControlOpts
	primaryZoneIndex int
	localIPs         []net.IP
	autoPathSearch   []string // Local search path from /etc/resolv.conf. Needed for autopath.
}

// Upstreamer is used to resolve CNAME or other external targets
type Upstreamer interface {
	Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error)
}

// New returns a initialized Kubernetes. It default interfaceAddrFunc to return 127.0.0.1. All other
// values default to their zero value, primaryZoneIndex will thus point to the first zone.
func New(zones []string) *Kubernetes {
	k := new(Kubernetes)
	k.Zones = zones
	k.Namespaces = make(map[string]struct{})
	k.podMode = podModeDisabled
	k.ttl = defaultTTL

	return k
}

const (
	// podModeDisabled is the default value where pod requests are ignored
	podModeDisabled = "disabled"
	// podModeVerified is where Pod requests are answered only if they exist
	podModeVerified = "verified"
	// podModeInsecure is where pod requests are answered without verifying they exist
	podModeInsecure = "insecure"
	// DNSSchemaVersion is the schema version: https://github.com/kubernetes/dns/blob/master/docs/specification.md
	DNSSchemaVersion = "1.1.0"
	// Svc is the DNS schema for kubernetes services
	Svc = "svc"
	// Pod is the DNS schema for kubernetes pods
	Pod = "pod"
	// defaultTTL to apply to all answers.
	defaultTTL = 5
)

var (
	errNoItems        = errors.New("no items found")
	errNsNotExposed   = errors.New("namespace is not exposed")
	errInvalidRequest = errors.New("invalid query name")
)

// Services implements the ServiceBackend interface.
func (k *Kubernetes) Services(ctx context.Context, state request.Request, exact bool, opt plugin.Options) (svcs []msg.Service, err error) {
	// We're looking again at types, which we've already done in ServeDNS, but there are some types k8s just can't answer.
	switch state.QType() {
	case dns.TypeTXT:
		// 1 label + zone, label must be "dns-version".
		t, _ := dnsutil.TrimZone(state.Name(), state.Zone)

		// Hard code the only valid TXT - "dns-version.<zone>"
		segs := dns.SplitDomainName(t)
		if len(segs) == 1 && segs[0] == "dns-version" {
			svc := msg.Service{Text: DNSSchemaVersion, TTL: 28800, Key: msg.Path(state.QName(), coredns)}
			return []msg.Service{svc}, nil
		}

		// Check if we have an existing record for this query of another type
		services, _ := k.Records(ctx, state, false)

		if len(services) > 0 {
			// If so we return an empty NOERROR
			return nil, nil
		}

		// Return NXDOMAIN for no match
		return nil, errNoItems

	case dns.TypeNS:
		// We can only get here if the qname equals the zone, see ServeDNS in handler.go.
		nss := k.nsAddrs(false, false, state.Zone)
		var svcs []msg.Service
		for _, ns := range nss {
			if ns.Header().Rrtype == dns.TypeA {
				svcs = append(svcs, msg.Service{Host: ns.(*dns.A).A.String(), Key: msg.Path(ns.Header().Name, coredns), TTL: k.ttl})
				continue
			}
			if ns.Header().Rrtype == dns.TypeAAAA {
				svcs = append(svcs, msg.Service{Host: ns.(*dns.AAAA).AAAA.String(), Key: msg.Path(ns.Header().Name, coredns), TTL: k.ttl})
			}
		}
		return svcs, nil
	}

	if isDefaultNS(state.Name(), state.Zone) {
		nss := k.nsAddrs(false, false, state.Zone)
		var svcs []msg.Service
		for _, ns := range nss {
			if ns.Header().Rrtype == dns.TypeA && state.QType() == dns.TypeA {
				svcs = append(svcs, msg.Service{Host: ns.(*dns.A).A.String(), Key: msg.Path(state.QName(), coredns), TTL: k.ttl})
				continue
			}
			if ns.Header().Rrtype == dns.TypeAAAA && state.QType() == dns.TypeAAAA {
				svcs = append(svcs, msg.Service{Host: ns.(*dns.AAAA).AAAA.String(), Key: msg.Path(state.QName(), coredns), TTL: k.ttl})
			}
		}
		return svcs, nil
	}

	s, e := k.Records(ctx, state, false)

	// SRV for external services is not yet implemented, so remove those records.

	if state.QType() != dns.TypeSRV {
		return s, e
	}

	internal := []msg.Service{}
	for _, svc := range s {
		if t, _ := svc.HostType(); t != dns.TypeCNAME {
			internal = append(internal, svc)
		}
	}

	return internal, e
}

// primaryZone will return the first non-reverse zone being handled by this plugin
func (k *Kubernetes) primaryZone() string { return k.Zones[k.primaryZoneIndex] }

// Lookup implements the ServiceBackend interface.
func (k *Kubernetes) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return k.Upstream.Lookup(ctx, state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (k *Kubernetes) IsNameError(err error) bool {
	return err == errNoItems || err == errNsNotExposed || err == errInvalidRequest
}

func (k *Kubernetes) getClientConfig() (*rest.Config, error) {
	if k.ClientConfig != nil {
		return k.ClientConfig.ClientConfig()
	}
	loadingRules := &clientcmd.ClientConfigLoadingRules{}
	overrides := &clientcmd.ConfigOverrides{}
	clusterinfo := clientcmdapi.Cluster{}
	authinfo := clientcmdapi.AuthInfo{}

	// Connect to API from in cluster
	if len(k.APIServerList) == 0 {
		cc, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		cc.ContentType = "application/vnd.kubernetes.protobuf"
		cc.UserAgent = fmt.Sprintf("%s/%s git_commit:%s (%s/%s/%s)", coremain.CoreName, coremain.CoreVersion, coremain.GitCommit, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return cc, err
	}

	// Connect to API from out of cluster
	// Only the first one is used. We will deprecate multiple endpoints later.
	clusterinfo.Server = k.APIServerList[0]

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

	cc, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	cc.ContentType = "application/vnd.kubernetes.protobuf"
	cc.UserAgent = fmt.Sprintf("%s/%s git_commit:%s (%s/%s/%s)", coremain.CoreName, coremain.CoreVersion, coremain.GitCommit, runtime.GOOS, runtime.GOARCH, runtime.Version())
	return cc, err
}

// InitKubeCache initializes a new Kubernetes cache.
func (k *Kubernetes) InitKubeCache(ctx context.Context) (onStart func() error, onShut func() error, err error) {
	config, err := k.getClientConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes notification controller: %q", err)
	}

	if k.opts.labelSelector != nil {
		var selector labels.Selector
		selector, err = meta.LabelSelectorAsSelector(k.opts.labelSelector)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to create Selector for LabelSelector '%s': %q", k.opts.labelSelector, err)
		}
		k.opts.selector = selector
	}

	if k.opts.namespaceLabelSelector != nil {
		var selector labels.Selector
		selector, err = meta.LabelSelectorAsSelector(k.opts.namespaceLabelSelector)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to create Selector for LabelSelector '%s': %q", k.opts.namespaceLabelSelector, err)
		}
		k.opts.namespaceSelector = selector
	}

	k.opts.initPodCache = k.podMode == podModeVerified

	k.opts.zones = k.Zones
	k.opts.endpointNameMode = k.endpointNameMode

	k.APIConn = newdnsController(ctx, kubeClient, k.opts)

	onStart = func() error {
		go func() {
			k.APIConn.Run()
		}()

		timeout := 5 * time.Second
		timeoutTicker := time.NewTicker(timeout)
		defer timeoutTicker.Stop()
		logDelay := 500 * time.Millisecond
		logTicker := time.NewTicker(logDelay)
		defer logTicker.Stop()
		checkSyncTicker := time.NewTicker(100 * time.Millisecond)
		defer checkSyncTicker.Stop()
		for {
			select {
			case <-checkSyncTicker.C:
				if k.APIConn.HasSynced() {
					return nil
				}
			case <-logTicker.C:
				log.Info("waiting for Kubernetes API before starting server")
			case <-timeoutTicker.C:
				log.Warning("starting server with unsynced Kubernetes API")
				return nil
			}
		}
	}

	onShut = func() error {
		return k.APIConn.Stop()
	}

	return onStart, onShut, err
}

// Records looks up services in kubernetes.
func (k *Kubernetes) Records(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error) {
	r, e := parseRequest(state.Name(), state.Zone)
	if e != nil {
		return nil, e
	}
	if r.podOrSvc == "" {
		return nil, nil
	}

	if dnsutil.IsReverse(state.Name()) > 0 {
		return nil, errNoItems
	}

	if !k.namespaceExposed(r.namespace) {
		return nil, errNsNotExposed
	}

	if r.podOrSvc == Pod {
		pods, err := k.findPods(r, state.Zone)
		return pods, err
	}

	services, err := k.findServices(r, state.Zone)
	return services, err
}

func endpointHostname(addr object.EndpointAddress, endpointNameMode bool) string {
	if addr.Hostname != "" {
		return addr.Hostname
	}
	if endpointNameMode && addr.TargetRefName != "" {
		return addr.TargetRefName
	}
	if strings.Contains(addr.IP, ".") {
		return strings.Replace(addr.IP, ".", "-", -1)
	}
	if strings.Contains(addr.IP, ":") {
		return strings.Replace(addr.IP, ":", "-", -1)
	}
	return ""
}

func (k *Kubernetes) findPods(r recordRequest, zone string) (pods []msg.Service, err error) {
	if k.podMode == podModeDisabled {
		return nil, errNoItems
	}

	namespace := r.namespace
	if !k.namespaceExposed(namespace) {
		return nil, errNoItems
	}

	podname := r.service

	// handle empty pod name
	if podname == "" {
		if k.namespaceExposed(namespace) {
			// NODATA
			return nil, nil
		}
		// NXDOMAIN
		return nil, errNoItems
	}

	zonePath := msg.Path(zone, coredns)
	ip := ""
	if strings.Count(podname, "-") == 3 && !strings.Contains(podname, "--") {
		ip = strings.ReplaceAll(podname, "-", ".")
	} else {
		ip = strings.ReplaceAll(podname, "-", ":")
	}

	if k.podMode == podModeInsecure {
		if !k.namespaceExposed(namespace) { // namespace does not exist
			return nil, errNoItems
		}

		// If ip does not parse as an IP address, we return an error, otherwise we assume a CNAME and will try to resolve it in backend_lookup.go
		if net.ParseIP(ip) == nil {
			return nil, errNoItems
		}

		return []msg.Service{{Key: strings.Join([]string{zonePath, Pod, namespace, podname}, "/"), Host: ip, TTL: k.ttl}}, err
	}

	// PodModeVerified
	err = errNoItems

	for _, p := range k.APIConn.PodIndex(ip) {
		// check for matching ip and namespace
		if ip == p.PodIP && match(namespace, p.Namespace) {
			s := msg.Service{Key: strings.Join([]string{zonePath, Pod, namespace, podname}, "/"), Host: ip, TTL: k.ttl}
			pods = append(pods, s)

			err = nil
		}
	}
	return pods, err
}

// findServices returns the services matching r from the cache.
func (k *Kubernetes) findServices(r recordRequest, zone string) (services []msg.Service, err error) {
	if !k.namespaceExposed(r.namespace) {
		return nil, errNoItems
	}

	// handle empty service name
	if r.service == "" {
		if k.namespaceExposed(r.namespace) {
			// NODATA
			return nil, nil
		}
		// NXDOMAIN
		return nil, errNoItems
	}

	err = errNoItems

	var (
		endpointsListFunc func() []*object.Endpoints
		endpointsList     []*object.Endpoints
		serviceList       []*object.Service
	)

	idx := object.ServiceKey(r.service, r.namespace)
	serviceList = k.APIConn.SvcIndex(idx)
	endpointsListFunc = func() []*object.Endpoints { return k.APIConn.EpIndex(idx) }

	zonePath := msg.Path(zone, coredns)
	for _, svc := range serviceList {
		if !(match(r.namespace, svc.Namespace) && match(r.service, svc.Name)) {
			continue
		}

		// If "ignore empty_service" option is set and no endpoints exist, return NXDOMAIN unless
		// it's a headless or externalName service (covered below).
		if k.opts.ignoreEmptyService && svc.Type != api.ServiceTypeExternalName && !svc.Headless() { // serve NXDOMAIN if no endpoint is able to answer
			podsCount := 0
			for _, ep := range endpointsListFunc() {
				for _, eps := range ep.Subsets {
					podsCount += len(eps.Addresses)
				}
			}

			if podsCount == 0 {
				continue
			}
		}

		// External service
		if svc.Type == api.ServiceTypeExternalName {
			// External services do not have endpoints, nor can we accept port/protocol pseudo subdomains in an SRV query, so skip this service if endpoint, port, or protocol is non-empty in the request
			if r.endpoint != "" || r.port != "" || r.protocol != "" {
				continue
			}
			s := msg.Service{Key: strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/"), Host: svc.ExternalName, TTL: k.ttl}
			if t, _ := s.HostType(); t == dns.TypeCNAME {
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")
				services = append(services, s)

				err = nil
			}
			continue
		}

		// Endpoint query or headless service
		if svc.Headless() || r.endpoint != "" {
			if endpointsList == nil {
				endpointsList = endpointsListFunc()
			}

			for _, ep := range endpointsList {
				if object.EndpointsKey(svc.Name, svc.Namespace) != ep.Index {
					continue
				}

				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						// See comments in parse.go parseRequest about the endpoint handling.
						if r.endpoint != "" {
							if !match(r.endpoint, endpointHostname(addr, k.endpointNameMode)) {
								continue
							}
						}

						for _, p := range eps.Ports {
							if !(matchPortAndProtocol(r.port, p.Name, r.protocol, p.Protocol)) {
								continue
							}
							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: k.ttl}
							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, endpointHostname(addr, k.endpointNameMode)}, "/")

							err = nil

							services = append(services, s)
						}
					}
				}
			}
			continue
		}

		// ClusterIP service
		for _, p := range svc.Ports {
			if !(matchPortAndProtocol(r.port, p.Name, r.protocol, string(p.Protocol))) {
				continue
			}

			err = nil

			for _, ip := range svc.ClusterIPs {
				s := msg.Service{Host: ip, Port: int(p.Port), TTL: k.ttl}
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")
				services = append(services, s)
			}
		}
	}
	return services, err
}

// Serial return the SOA serial.
func (k *Kubernetes) Serial(state request.Request) uint32 { return uint32(k.APIConn.Modified(false)) }

// MinTTL returns the minimal TTL.
func (k *Kubernetes) MinTTL(state request.Request) uint32 { return k.ttl }

// match checks if a and b are equal.
func match(a, b string) bool {
	return strings.EqualFold(a, b)
}

// matchPortAndProtocol matches port and protocol, permitting the 'a' inputs to be wild
func matchPortAndProtocol(aPort, bPort, aProtocol, bProtocol string) bool {
	return (match(aPort, bPort) || aPort == "") && (match(aProtocol, bProtocol) || aProtocol == "")
}

const coredns = "c" // used as a fake key prefix in msg.Service
