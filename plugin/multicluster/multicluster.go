package multicluster

import (
	"context"
	"errors"
	"fmt"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"
	"github.com/coredns/coredns/plugin/multicluster/object"
	model "github.com/coredns/coredns/plugin/multicluster/object"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	apiv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/mcs-api/pkg/client/clientset/versioned/typed/apis/v1alpha1"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

const (
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

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin(pluginName)

// MultiCluster implements a plugin supporting multi-cluster DNS spec.
type MultiCluster struct {
	Next         plugin.Handler
	Zones        []string
	ClientConfig clientcmd.ClientConfig
	Fall         fall.F
	controller   controller
	ttl          uint32
	opts         controllerOpts
}

func New(zones []string) *MultiCluster {
	m := MultiCluster{
		Zones: zones,
	}

	m.ttl = defaultTTL

	return &m
}

func (m *MultiCluster) InitController(ctx context.Context) (onStart func() error, onShut func() error, err error) {
	config, err := m.getClientConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes notification controller: %q", err)
	}

	mcsClient, err := v1alpha1.NewForConfig(config)

	m.controller = newController(ctx, kubeClient, mcsClient, m.opts)

	onStart = func() error {
		go func() {
			m.controller.Run()
		}()

		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if m.controller.HasSynced() {
					return nil
				}
			case <-timeout:
				log.Warning("starting server with unsynced Kubernetes API")
				return nil
			}
		}
	}

	onShut = func() error {
		return m.controller.Stop()
	}

	return onStart, onShut, err
}

func (m MultiCluster) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.QName()

	zone := plugin.Zones(m.Zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)
	}
	zone = qname[len(qname)-len(zone):] // maintain case of original query
	state.Zone = zone

	var (
		records []dns.RR
		extra   []dns.RR
		err     error
	)

	switch state.QType() {
	case dns.TypeA:
		records, err = plugin.A(ctx, &m, zone, state, nil, plugin.Options{})
	case dns.TypeAAAA:
		records, err = plugin.AAAA(ctx, &m, zone, state, nil, plugin.Options{})
	case dns.TypeTXT:
		records, err = plugin.TXT(ctx, &m, zone, state, nil, plugin.Options{})
	case dns.TypeSRV:
		records, extra, err = plugin.SRV(ctx, &m, zone, state, plugin.Options{})
	case dns.TypeNS:
		if state.Name() == zone {
			records, extra, err = plugin.NS(ctx, &m, zone, state, plugin.Options{})
			break
		}
		fallthrough
	default:
		// Do a fake A lookup, so we can distinguish between NODATA and NXDOMAIN
		fake := state.NewWithQuestion(state.QName(), dns.TypeA)
		fake.Zone = state.Zone
		_, err = plugin.A(ctx, &m, zone, fake, nil, plugin.Options{})
	}

	if m.IsNameError(err) {
		if m.Fall.Through(state.Name()) {
			return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)
		}
		if !m.controller.HasSynced() {
			// If we haven't synchronized with the kubernetes cluster, return server failure
			return plugin.BackendError(ctx, &m, zone, dns.RcodeServerFailure, state, nil /* err */, plugin.Options{})
		}
		return plugin.BackendError(ctx, &m, zone, dns.RcodeNameError, state, nil /* err */, plugin.Options{})
	}
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	if len(records) == 0 {
		return plugin.BackendError(ctx, &m, zone, dns.RcodeSuccess, state, nil, plugin.Options{})
	}

	message := new(dns.Msg)
	message.SetReply(r)
	message.Authoritative = true
	message.Answer = append(message.Answer, records...)
	message.Extra = append(message.Extra, extra...)
	w.WriteMsg(message)
	return dns.RcodeSuccess, nil

}

// Name implements the Handler interface.
func (m MultiCluster) Name() string { return pluginName }

// Implement ServiceBackend interface

// Services communicates with the backend to retrieve the service definitions. Exact indicates
// on exact match should be returned.
func (m MultiCluster) Services(ctx context.Context, state request.Request, exact bool, opt plugin.Options) ([]msg.Service, error) {
	switch state.QType() {

	case dns.TypeTXT:
		// 1 label + zone, label must be "dns-version".
		t, _ := dnsutil.TrimZone(state.Name(), state.Zone)

		segs := dns.SplitDomainName(t)
		if len(segs) != 1 {
			return nil, nil
		}
		if segs[0] != "dns-version" {
			return nil, nil
		}
		svc := msg.Service{Text: DNSSchemaVersion, TTL: 28800, Key: msg.Path(state.QName(), coredns)}
		return []msg.Service{svc}, nil

		// TODO support TypeNS
	}

	return m.Records(ctx, state, false)
}

// Reverse communicates with the backend to retrieve service definition based on a IP address
// instead of a name. I.e. a reverse DNS lookup.
func (m MultiCluster) Reverse(ctx context.Context, state request.Request, exact bool, opt plugin.Options) ([]msg.Service, error) {
	return nil, errors.New("reverse lookup is not supported")
}

// Lookup is used to find records else where.
func (m MultiCluster) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return nil, errors.New("external lookup is not supported")
}

// Returns _all_ services that matches a certain name.
// Note: it does not implement a specific service.
func (m MultiCluster) Records(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error) {
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

	if !wildcard(r.namespace) && !m.namespaceExists(r.namespace) {
		return nil, errNsNotExposed
	}

	services, err := m.findServices(r, state.Zone)
	return services, err
}

// IsNameError returns true if err indicated a record not found condition
func (m MultiCluster) IsNameError(err error) bool {
	return err == errNoItems || err == errNsNotExposed || err == errInvalidRequest
}

// Serial returns a SOA serial number to construct a SOA record.
func (m MultiCluster) Serial(state request.Request) uint32 {
	return uint32(m.controller.Modified())
}

// MinTTL returns the minimum TTL to be used in the SOA record.
func (m MultiCluster) MinTTL(state request.Request) uint32 {
	return m.ttl
}

type ResponsePrinter struct {
	dns.ResponseWriter
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter) *ResponsePrinter {
	return &ResponsePrinter{ResponseWriter: w}
}

func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	return r.ResponseWriter.WriteMsg(res)
}

func (m *MultiCluster) getClientConfig() (*rest.Config, error) {
	if m.ClientConfig != nil {
		return m.ClientConfig.ClientConfig()
	}

	cc, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	cc.ContentType = "application/vnd.kubernetes.protobuf"
	return cc, err
}

// wildcard checks whether s contains a wildcard value defined as "*" or "any".
func wildcard(s string) bool {
	return s == "*" || s == "any"
}

func (m *MultiCluster) namespaceExists(namespace string) bool {
	_, err := m.controller.GetNamespaceByName(namespace)
	if err != nil {
		return false
	}
	return true
}

func (m *MultiCluster) findServices(r recordRequest, zone string) (services []msg.Service, err error) {
	if !wildcard(r.namespace) && !m.namespaceExists(r.namespace) {
		return nil, errNoItems
	}

	// handle empty service name
	if r.service == "" {
		if m.namespaceExists(r.namespace) || wildcard(r.namespace) {
			// NODATA
			return nil, nil
		}
		// NXDOMAIN
		return nil, errNoItems
	}

	err = errNoItems
	if wildcard(r.service) && !wildcard(r.namespace) {
		// If namespace exists, err should be nil, so that we return NODATA instead of NXDOMAIN
		if m.namespaceExists(r.namespace) {
			err = nil
		}
	}

	var (
		endpointsListFunc func() []*object.Endpoints
		endpointsList     []*object.Endpoints
		serviceList       []*model.ServiceImport
	)

	if wildcard(r.service) || wildcard(r.namespace) {
		serviceList = m.controller.ServiceList()
		endpointsListFunc = func() []*object.Endpoints { return m.controller.EndpointsList() }
	} else {
		idx := object.ServiceKey(r.service, r.namespace)
		serviceList = m.controller.SvcIndex(idx)
		endpointsListFunc = func() []*object.Endpoints { return m.controller.EpIndex(idx) }
	}

	zonePath := msg.Path(zone, coredns)
	for _, svc := range serviceList {
		if !(match(r.namespace, svc.Namespace) && match(r.service, svc.Name)) {
			continue
		}

		// If request namespace is a wildcard, filter results against Corefile namespace list.
		// (Namespaces without a wildcard were filtered before the call to this function.)
		if wildcard(r.namespace) && !m.namespaceExists(svc.Namespace) {
			continue
		}

		// Headless service or endpoint query
		if svc.Type == apiv1alpha1.Headless || r.endpoint != "" {
			if endpointsList == nil {
				endpointsList = endpointsListFunc()
			}

			for _, ep := range endpointsList {
				if object.EndpointsKey(svc.Name, svc.Namespace) != ep.Index {
					continue
				}

				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {

						if r.endpoint != "" {
							if !match(r.cluster, ep.ClusterId) || !match(r.endpoint, endpointHostname(addr)) {
								continue
							}
						}

						for _, p := range eps.Ports {
							if !(match(r.port, p.Name) && match(r.protocol, string(p.Protocol))) {
								continue
							}
							s := msg.Service{Host: addr.IP, Port: int(p.Port), TTL: m.ttl}
							s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name, ep.ClusterId, endpointHostname(addr)}, "/")

							err = nil

							services = append(services, s)
						}
					}
				}
			}
			continue
		}

		// ClusterSetIP service
		for _, p := range svc.Ports {
			if !(match(r.port, p.Name) && match(r.protocol, string(p.Protocol))) {
				continue
			}

			err = nil

			for _, ip := range svc.ClusterIPs {
				s := msg.Service{Host: ip, Port: int(p.Port), TTL: m.ttl}
				s.Key = strings.Join([]string{zonePath, Svc, svc.Namespace, svc.Name}, "/")
				services = append(services, s)
			}
		}
	}
	return services, err
}

func endpointHostname(addr object.EndpointAddress) string {
	if addr.Hostname != "" {
		return addr.Hostname
	}
	if strings.Contains(addr.IP, ".") {
		return strings.Replace(addr.IP, ".", "-", -1)
	}
	if strings.Contains(addr.IP, ":") {
		return strings.Replace(addr.IP, ":", "-", -1)
	}
	return ""
}

// match checks if a and b are equal taking wildcards into account.
func match(a, b string) bool {
	if wildcard(a) {
		return true
	}
	if wildcard(b) {
		return true
	}
	return strings.EqualFold(a, b)
}

const coredns = "c" // used as a fake key prefix in msg.Service
