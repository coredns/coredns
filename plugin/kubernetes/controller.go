package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/kubernetes/object"

	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	podIPIndex            = "PodIP"
	svcNameNamespaceIndex = "ServiceNameNamespace"
	svcIPIndex            = "ServiceIP"
	svcExtIPIndex         = "ServiceExternalIP"
	epNameNamespaceIndex  = "EndpointNameNamespace"
	epIPIndex             = "EndpointsIP"
)

type dnsController interface {
	ServiceList() []*object.Service
	EndpointsList() []*object.Endpoints
	SvcIndex(string) []*object.Service
	SvcIndexReverse(string) []*object.Service
	SvcExtIndexReverse(string) []*object.Service
	PodIndex(string) []*object.Pod
	EpIndex(string) []*object.Endpoints
	EpIndexReverse(string) []*object.Endpoints

	GetNodeByName(context.Context, string) (*api.Node, error)
	GetNamespaceByName(string) (*object.Namespace, error)

	Run()
	HasSynced() bool
	Stop() error

	// Modified returns the timestamp of the most recent changes to services.  If the passed bool is true, it should
	// return the timestamp of the most recent changes to services with external facing IP addresses
	Modified(bool) int64
}

type dnsControl struct {
	// modified tracks timestamp of the most recent changes
	// It needs to be first because it is guaranteed to be 8-byte
	// aligned ( we use sync.LoadAtomic with this )
	modified int64
	// extModified tracks timestamp of the most recent changes to
	// services with external facing IP addresses
	extModified int64

	client kubernetes.Interface

	selector          labels.Selector
	namespaceSelector labels.Selector

	svcController cache.Controller
	podController cache.Controller
	epController  cache.Controller
	nsController  cache.Controller

	svcLister cache.Indexer
	podLister cache.Indexer
	epLister  cache.Indexer
	nsLister  cache.Store

	// stopLock is used to enforce only a single call to Stop is active.
	// Needed because we allow stopping through an http endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}

	zones            []string
	endpointNameMode bool
}

type dnsControlOpts struct {
	initPodCache       bool
	initEndpointsCache bool
	ignoreEmptyService bool

	// Label handling.
	labelSelector          *meta.LabelSelector
	selector               labels.Selector
	namespaceLabelSelector *meta.LabelSelector
	namespaceSelector      labels.Selector

	zones            []string
	endpointNameMode bool
}

// newdnsController creates a controller for CoreDNS.
func newdnsController(ctx context.Context, kubeClient kubernetes.Interface, opts dnsControlOpts) *dnsControl {
	dns := dnsControl{
		client:            kubeClient,
		selector:          opts.selector,
		namespaceSelector: opts.namespaceSelector,
		stopCh:            make(chan struct{}),
		zones:             opts.zones,
		endpointNameMode:  opts.endpointNameMode,
	}

	dns.svcLister, dns.svcController = object.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  serviceListFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
			WatchFunc: serviceWatchFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
		},
		&api.Service{},
		cache.ResourceEventHandlerFuncs{AddFunc: dns.Add, UpdateFunc: dns.Update, DeleteFunc: dns.Delete},
		cache.Indexers{svcNameNamespaceIndex: svcNameNamespaceIndexFunc, svcIPIndex: svcIPIndexFunc, svcExtIPIndex: svcExtIPIndexFunc},
		object.DefaultProcessor(object.ToService, nil),
	)

	podLister, podController := object.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  podListFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
			WatchFunc: podWatchFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
		},
		&api.Pod{},
		cache.ResourceEventHandlerFuncs{AddFunc: dns.Add, UpdateFunc: dns.Update, DeleteFunc: dns.Delete},
		cache.Indexers{podIPIndex: podIPIndexFunc},
		object.DefaultProcessor(object.ToPod, nil),
	)
	dns.podLister = podLister
	if opts.initPodCache {
		dns.podController = podController
	}

	epLister, epController := object.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  endpointSliceListFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
			WatchFunc: endpointSliceWatchFunc(ctx, dns.client, api.NamespaceAll, dns.selector),
		},
		&discovery.EndpointSlice{},
		cache.ResourceEventHandlerFuncs{AddFunc: dns.Add, UpdateFunc: dns.Update, DeleteFunc: dns.Delete},
		cache.Indexers{epNameNamespaceIndex: epNameNamespaceIndexFunc, epIPIndex: epIPIndexFunc},
		object.DefaultProcessor(object.EndpointSliceToEndpoints, dns.EndpointSliceLatencyRecorder()),
	)
	dns.epLister = epLister
	if opts.initEndpointsCache {
		dns.epController = epController
	}

	dns.nsLister, dns.nsController = object.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  namespaceListFunc(ctx, dns.client, dns.namespaceSelector),
			WatchFunc: namespaceWatchFunc(ctx, dns.client, dns.namespaceSelector),
		},
		&api.Namespace{},
		cache.ResourceEventHandlerFuncs{},
		cache.Indexers{},
		object.DefaultProcessor(object.ToNamespace, nil),
	)

	return &dns
}

func (dns *dnsControl) EndpointsLatencyRecorder() *object.EndpointLatencyRecorder {
	return &object.EndpointLatencyRecorder{
		ServiceFunc: func(o meta.Object) []*object.Service {
			return dns.SvcIndex(object.ServiceKey(o.GetName(), o.GetNamespace()))
		},
	}
}
func (dns *dnsControl) EndpointSliceLatencyRecorder() *object.EndpointLatencyRecorder {
	return &object.EndpointLatencyRecorder{
		ServiceFunc: func(o meta.Object) []*object.Service {
			return dns.SvcIndex(object.ServiceKey(o.GetLabels()[discovery.LabelServiceName], o.GetNamespace()))
		},
	}
}

func podIPIndexFunc(obj interface{}) ([]string, error) {
	p, ok := obj.(*object.Pod)
	if !ok {
		return nil, errObj
	}
	return []string{p.PodIP}, nil
}

func svcIPIndexFunc(obj interface{}) ([]string, error) {
	svc, ok := obj.(*object.Service)
	if !ok {
		return nil, errObj
	}
	idx := make([]string, len(svc.ClusterIPs))
	copy(idx, svc.ClusterIPs)
	return idx, nil
}

func svcExtIPIndexFunc(obj interface{}) ([]string, error) {
	svc, ok := obj.(*object.Service)
	if !ok {
		return nil, errObj
	}
	idx := make([]string, len(svc.ExternalIPs))
	copy(idx, svc.ExternalIPs)
	return idx, nil
}

func svcNameNamespaceIndexFunc(obj interface{}) ([]string, error) {
	s, ok := obj.(*object.Service)
	if !ok {
		return nil, errObj
	}
	return []string{s.Index}, nil
}

func epNameNamespaceIndexFunc(obj interface{}) ([]string, error) {
	s, ok := obj.(*object.Endpoints)
	if !ok {
		return nil, errObj
	}
	return []string{s.Index}, nil
}

func epIPIndexFunc(obj interface{}) ([]string, error) {
	ep, ok := obj.(*object.Endpoints)
	if !ok {
		return nil, errObj
	}
	return ep.IndexIP, nil
}

func serviceListFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		if s != nil {
			opts.LabelSelector = s.String()
		}
		return c.CoreV1().Services(ns).List(ctx, opts)
	}
}

func podListFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		if s != nil {
			opts.LabelSelector = s.String()
		}
		if len(opts.FieldSelector) > 0 {
			opts.FieldSelector = opts.FieldSelector + ","
		}
		opts.FieldSelector = opts.FieldSelector + "status.phase!=Succeeded,status.phase!=Failed,status.phase!=Unknown"
		return c.CoreV1().Pods(ns).List(ctx, opts)
	}
}

func endpointSliceListFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		if s != nil {
			opts.LabelSelector = s.String()
		}
		return c.DiscoveryV1().EndpointSlices(ns).List(ctx, opts)
	}
}

func namespaceListFunc(ctx context.Context, c kubernetes.Interface, s labels.Selector) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		if s != nil {
			opts.LabelSelector = s.String()
		}
		return c.CoreV1().Namespaces().List(ctx, opts)
	}
}

func serviceWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		return c.CoreV1().Services(ns).Watch(ctx, options)
	}
}

func podWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		if len(options.FieldSelector) > 0 {
			options.FieldSelector = options.FieldSelector + ","
		}
		options.FieldSelector = options.FieldSelector + "status.phase!=Succeeded,status.phase!=Failed,status.phase!=Unknown"
		return c.CoreV1().Pods(ns).Watch(ctx, options)
	}
}

func endpointSliceWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		return c.DiscoveryV1().EndpointSlices(ns).Watch(ctx, options)
	}
}

func namespaceWatchFunc(ctx context.Context, c kubernetes.Interface, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		return c.CoreV1().Namespaces().Watch(ctx, options)
	}
}

// Stop stops the  controller.
func (dns *dnsControl) Stop() error {
	dns.stopLock.Lock()
	defer dns.stopLock.Unlock()

	// Only try draining the workqueue if we haven't already.
	if !dns.shutdown {
		close(dns.stopCh)
		dns.shutdown = true

		return nil
	}

	return fmt.Errorf("shutdown already in progress")
}

// Run starts the controller.
func (dns *dnsControl) Run() {
	go dns.svcController.Run(dns.stopCh)
	if dns.epController != nil {
		go func() {
			dns.epController.Run(dns.stopCh)
		}()
	}
	if dns.podController != nil {
		go dns.podController.Run(dns.stopCh)
	}
	go dns.nsController.Run(dns.stopCh)
	<-dns.stopCh
}

// HasSynced calls on all controllers.
func (dns *dnsControl) HasSynced() bool {
	a := dns.svcController.HasSynced()
	b := true
	if dns.epController != nil {
		b = dns.epController.HasSynced()
	}
	c := true
	if dns.podController != nil {
		c = dns.podController.HasSynced()
	}
	d := dns.nsController.HasSynced()
	return a && b && c && d
}

func (dns *dnsControl) ServiceList() (svcs []*object.Service) {
	os := dns.svcLister.List()
	for _, o := range os {
		s, ok := o.(*object.Service)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (dns *dnsControl) EndpointsList() (eps []*object.Endpoints) {
	os := dns.epLister.List()
	for _, o := range os {
		ep, ok := o.(*object.Endpoints)
		if !ok {
			continue
		}
		eps = append(eps, ep)
	}
	return eps
}

func (dns *dnsControl) PodIndex(ip string) (pods []*object.Pod) {
	os, err := dns.podLister.ByIndex(podIPIndex, ip)
	if err != nil {
		return nil
	}
	for _, o := range os {
		p, ok := o.(*object.Pod)
		if !ok {
			continue
		}
		pods = append(pods, p)
	}
	return pods
}

func (dns *dnsControl) SvcIndex(idx string) (svcs []*object.Service) {
	os, err := dns.svcLister.ByIndex(svcNameNamespaceIndex, idx)
	if err != nil {
		return nil
	}
	for _, o := range os {
		s, ok := o.(*object.Service)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (dns *dnsControl) SvcIndexReverse(ip string) (svcs []*object.Service) {
	os, err := dns.svcLister.ByIndex(svcIPIndex, ip)
	if err != nil {
		return nil
	}

	for _, o := range os {
		s, ok := o.(*object.Service)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (dns *dnsControl) SvcExtIndexReverse(ip string) (svcs []*object.Service) {
	os, err := dns.svcLister.ByIndex(svcExtIPIndex, ip)
	if err != nil {
		return nil
	}

	for _, o := range os {
		s, ok := o.(*object.Service)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (dns *dnsControl) EpIndex(idx string) (ep []*object.Endpoints) {
	os, err := dns.epLister.ByIndex(epNameNamespaceIndex, idx)
	if err != nil {
		return nil
	}
	for _, o := range os {
		e, ok := o.(*object.Endpoints)
		if !ok {
			continue
		}
		ep = append(ep, e)
	}
	return ep
}

func (dns *dnsControl) EpIndexReverse(ip string) (ep []*object.Endpoints) {
	os, err := dns.epLister.ByIndex(epIPIndex, ip)
	if err != nil {
		return nil
	}
	for _, o := range os {
		e, ok := o.(*object.Endpoints)
		if !ok {
			continue
		}
		ep = append(ep, e)
	}
	return ep
}

// GetNodeByName return the node by name. If nothing is found an error is
// returned. This query causes a round trip to the k8s API server, so use
// sparingly. Currently, this is only used for Federation.
func (dns *dnsControl) GetNodeByName(ctx context.Context, name string) (*api.Node, error) {
	v1node, err := dns.client.CoreV1().Nodes().Get(ctx, name, meta.GetOptions{})
	return v1node, err
}

// GetNamespaceByName returns the namespace by name. If nothing is found an error is returned.
func (dns *dnsControl) GetNamespaceByName(name string) (*object.Namespace, error) {
	o, exists, err := dns.nsLister.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("namespace not found")
	}
	ns, ok := o.(*object.Namespace)
	if !ok {
		return nil, fmt.Errorf("found key but not namespace")
	}
	return ns, nil
}

func (dns *dnsControl) Add(obj interface{})               { dns.updateModified() }
func (dns *dnsControl) Delete(obj interface{})            { dns.updateModified() }
func (dns *dnsControl) Update(oldObj, newObj interface{}) { dns.detectChanges(oldObj, newObj) }

// detectChanges detects changes in objects, and updates the modified timestamp
func (dns *dnsControl) detectChanges(oldObj, newObj interface{}) {
	// If both objects have the same resource version, they are identical.
	if newObj != nil && oldObj != nil && (oldObj.(meta.Object).GetResourceVersion() == newObj.(meta.Object).GetResourceVersion()) {
		return
	}
	obj := newObj
	if obj == nil {
		obj = oldObj
	}
	switch ob := obj.(type) {
	case *object.Service:
		imod, emod := serviceModified(oldObj, newObj)
		if imod {
			dns.updateModified()
		}
		if emod {
			dns.updateExtModified()
		}
	case *object.Pod:
		dns.updateModified()
	case *object.Endpoints:
		if !endpointsEquivalent(oldObj.(*object.Endpoints), newObj.(*object.Endpoints)) {
			dns.updateModified()
		}
	default:
		log.Warningf("Updates for %T not supported.", ob)
	}
}

// subsetsEquivalent checks if two endpoint subsets are significantly equivalent
// I.e. that they have the same ready addresses, host names, ports (including protocol
// and service names for SRV)
func subsetsEquivalent(sa, sb object.EndpointSubset) bool {
	if len(sa.Addresses) != len(sb.Addresses) {
		return false
	}
	if len(sa.Ports) != len(sb.Ports) {
		return false
	}

	// in Addresses and Ports, we should be able to rely on
	// these being sorted and able to be compared
	// they are supposed to be in a canonical format
	for addr, aaddr := range sa.Addresses {
		baddr := sb.Addresses[addr]
		if aaddr.IP != baddr.IP {
			return false
		}
		if aaddr.Hostname != baddr.Hostname {
			return false
		}
	}

	for port, aport := range sa.Ports {
		bport := sb.Ports[port]
		if aport.Name != bport.Name {
			return false
		}
		if aport.Port != bport.Port {
			return false
		}
		if aport.Protocol != bport.Protocol {
			return false
		}
	}
	return true
}

// endpointsEquivalent checks if the update to an endpoint is something
// that matters to us or if they are effectively equivalent.
func endpointsEquivalent(a, b *object.Endpoints) bool {
	if a == nil || b == nil {
		return false
	}

	if len(a.Subsets) != len(b.Subsets) {
		return false
	}

	// we should be able to rely on
	// these being sorted and able to be compared
	// they are supposed to be in a canonical format
	for i, sa := range a.Subsets {
		sb := b.Subsets[i]
		if !subsetsEquivalent(sa, sb) {
			return false
		}
	}
	return true
}

// serviceModified checks the services passed for changes that result in changes
// to internal and or external records.  It returns two booleans, one for internal
// record changes, and a second for external record changes
func serviceModified(oldObj, newObj interface{}) (intSvc, extSvc bool) {
	if oldObj != nil && newObj == nil {
		// deleted service only modifies external zone records if it had external ips
		return true, len(oldObj.(*object.Service).ExternalIPs) > 0
	}

	if oldObj == nil && newObj != nil {
		// added service only modifies external zone records if it has external ips
		return true, len(newObj.(*object.Service).ExternalIPs) > 0
	}

	newSvc := newObj.(*object.Service)
	oldSvc := oldObj.(*object.Service)

	// External IPs are mutable, affecting external zone records
	if len(oldSvc.ExternalIPs) != len(newSvc.ExternalIPs) {
		extSvc = true
	} else {
		for i := range oldSvc.ExternalIPs {
			if oldSvc.ExternalIPs[i] != newSvc.ExternalIPs[i] {
				extSvc = true
				break
			}
		}
	}

	// ExternalName is mutable, affecting internal zone records
	intSvc = oldSvc.ExternalName != newSvc.ExternalName

	if intSvc && extSvc {
		return intSvc, extSvc
	}

	// All Port fields are mutable, affecting both internal/external zone records
	if len(oldSvc.Ports) != len(newSvc.Ports) {
		return true, true
	}
	for i := range oldSvc.Ports {
		if oldSvc.Ports[i].Name != newSvc.Ports[i].Name {
			return true, true
		}
		if oldSvc.Ports[i].Port != newSvc.Ports[i].Port {
			return true, true
		}
		if oldSvc.Ports[i].Protocol != newSvc.Ports[i].Protocol {
			return true, true
		}
	}

	return intSvc, extSvc
}

func (dns *dnsControl) Modified(external bool) int64 {
	if external {
		return atomic.LoadInt64(&dns.extModified)
	}
	return atomic.LoadInt64(&dns.modified)
}

// updateModified set dns.modified to the current time.
func (dns *dnsControl) updateModified() {
	unix := time.Now().Unix()
	atomic.StoreInt64(&dns.modified, unix)
}

// updateExtModified set dns.extModified to the current time.
func (dns *dnsControl) updateExtModified() {
	unix := time.Now().Unix()
	atomic.StoreInt64(&dns.extModified, unix)
}

var errObj = errors.New("obj was not of the correct type")
