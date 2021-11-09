package multicluster

import (
	"context"
	"errors"
	"fmt"
	k8s "github.com/coredns/coredns/plugin/kubernetes/object"
	"github.com/coredns/coredns/plugin/multicluster/object"
	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	mcs "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned/typed/apis/v1alpha1"
	"sync"
	"sync/atomic"
	"time"
)

const (
	svcNameNamespaceIndex = "ServiceNameNamespace"
	epNameNamespaceIndex  = "EndpointNameNamespace"
)

type controller interface {
	ServiceList() []*object.ServiceImport
	EndpointsList() []*object.Endpoints
	SvcIndex(string) []*object.ServiceImport
	EpIndex(string) []*object.Endpoints

	GetNamespaceByName(string) (*object.Namespace, error)

	Run()
	HasSynced() bool
	Stop() error

	// Modified returns the timestamp of the most recent changes
	Modified() int64
}

type control struct {
	// Modified tracks timestamp of the most recent changes
	// It needs to be first because it is guaranteed to be 8-byte
	// aligned ( we use sync.LoadAtomic with this )
	modified int64

	k8sClient kubernetes.Interface
	mcsClient mcs.MulticlusterV1alpha1Interface

	svcImportController cache.Controller
	svcImportLister     cache.Indexer

	nsController cache.Controller
	nsLister     cache.Store

	epController cache.Controller
	epLister     cache.Indexer

	// stopLock is used to enforce only a single call to Stop is active.
	// Needed because we allow stopping through an http endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}
}

type controllerOpts struct {
	initEndpointsCache bool
}

func newController(ctx context.Context, k8sClient kubernetes.Interface, mcsClient mcs.MulticlusterV1alpha1Interface, opts controllerOpts) *control {
	ctl := control{
		k8sClient: k8sClient,
		mcsClient: mcsClient,
		stopCh:    make(chan struct{}),
	}

	// enable ServiceImport watch
	ctl.watchServiceImport(ctx)

	// enable Namespace watch
	ctl.watchNamespace(ctx)

	if opts.initEndpointsCache {
		ctl.watchEndpointSlice(ctx)
	}

	return &ctl
}

func (c *control) watchServiceImport(ctx context.Context) {
	c.svcImportLister, c.svcImportController = k8s.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  serviceImportListFunc(ctx, c.mcsClient, api.NamespaceAll),
			WatchFunc: serviceImportWatchFunc(ctx, c.mcsClient, api.NamespaceAll),
		},
		&v1alpha1.ServiceImport{},
		cache.ResourceEventHandlerFuncs{AddFunc: c.Add, UpdateFunc: c.Update, DeleteFunc: c.Delete},
		cache.Indexers{svcNameNamespaceIndex: svcNameNamespaceIndexFunc},
		k8s.DefaultProcessor(object.ToServiceImport, nil),
	)
}

func (c *control) watchNamespace(ctx context.Context) {
	c.nsLister, c.nsController = k8s.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  namespaceListFunc(ctx, c.k8sClient),
			WatchFunc: namespaceWatchFunc(ctx, c.k8sClient),
		},
		&api.Namespace{},
		cache.ResourceEventHandlerFuncs{},
		cache.Indexers{},
		k8s.DefaultProcessor(object.ToNamespace, nil),
	)
}

func (c *control) watchEndpointSlice(ctx context.Context) {
	c.epLister, c.epController = k8s.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  endpointSliceListFunc(ctx, c.k8sClient, api.NamespaceAll),
			WatchFunc: endpointSliceWatchFunc(ctx, c.k8sClient, api.NamespaceAll),
		},
		&discovery.EndpointSlice{},
		cache.ResourceEventHandlerFuncs{AddFunc: c.Add, UpdateFunc: c.Update, DeleteFunc: c.Delete},
		cache.Indexers{epNameNamespaceIndex: epNameNamespaceIndexFunc},
		k8s.DefaultProcessor(object.EndpointSliceToEndpoints, nil),
	)
}

// watchEndpointSliceV1beta1 will set the endpoint Lister and Controller to watch v1beta1
// instead of the default v1.
func (c *control) WatchEndpointSliceV1beta1(ctx context.Context) {
	c.epLister, c.epController = k8s.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  endpointSliceListFuncV1beta1(ctx, c.k8sClient, api.NamespaceAll),
			WatchFunc: endpointSliceWatchFuncV1beta1(ctx, c.k8sClient, api.NamespaceAll),
		},
		&discoveryV1beta1.EndpointSlice{},
		cache.ResourceEventHandlerFuncs{AddFunc: c.Add, UpdateFunc: c.Update, DeleteFunc: c.Delete},
		cache.Indexers{epNameNamespaceIndex: epNameNamespaceIndexFunc},
		k8s.DefaultProcessor(object.EndpointSliceV1beta1ToEndpoints, nil),
	)
}

// Stop stops the  controller.
func (c *control) Stop() error {
	c.stopLock.Lock()
	defer c.stopLock.Unlock()

	// Only try draining the workqueue if we haven't already.
	if !c.shutdown {
		close(c.stopCh)
		c.shutdown = true

		return nil
	}

	return fmt.Errorf("shutdown already in progress")
}

// Run starts the controller.
func (c *control) Run() {
	go c.svcImportController.Run(c.stopCh)
	go c.nsController.Run(c.stopCh)
	if c.epController != nil {
		c.epController.Run(c.stopCh)
	}

	<-c.stopCh
}

// HasSynced calls on all controllers.
func (c *control) HasSynced() bool {
	return c.svcImportController.HasSynced() && c.nsController.HasSynced()
}

func (c *control) SvcIndex(idx string) (svcs []*object.ServiceImport) {
	os, err := c.svcImportLister.ByIndex(svcNameNamespaceIndex, idx)
	if err != nil {
		return nil
	}
	for _, o := range os {
		s, ok := o.(*object.ServiceImport)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (c *control) ServiceList() (svcs []*object.ServiceImport) {
	os := c.svcImportLister.List()
	for _, o := range os {
		s, ok := o.(*object.ServiceImport)
		if !ok {
			continue
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func (c *control) EndpointsList() (eps []*object.Endpoints) {
	os := c.epLister.List()
	for _, o := range os {
		ep, ok := o.(*object.Endpoints)
		if !ok {
			continue
		}
		eps = append(eps, ep)
	}
	return eps
}

func (c *control) EpIndex(idx string) (ep []*object.Endpoints) {
	os, err := c.epLister.ByIndex(epNameNamespaceIndex, idx)
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

func serviceImportListFunc(ctx context.Context, c mcs.MulticlusterV1alpha1Interface, ns string) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		return c.ServiceImports(ns).List(ctx, opts)
	}
}

func serviceImportWatchFunc(ctx context.Context, c mcs.MulticlusterV1alpha1Interface, ns string) func(options meta.ListOptions) (watch.Interface, error) {
	return func(opts meta.ListOptions) (watch.Interface, error) {
		return c.ServiceImports(ns).Watch(ctx, opts)
	}
}

func namespaceListFunc(ctx context.Context, c kubernetes.Interface) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		return c.CoreV1().Namespaces().List(ctx, opts)
	}
}

func namespaceWatchFunc(ctx context.Context, c kubernetes.Interface) func(options meta.ListOptions) (watch.Interface, error) {
	return func(opts meta.ListOptions) (watch.Interface, error) {
		return c.CoreV1().Namespaces().Watch(ctx, opts)
	}
}

func endpointSliceListFunc(ctx context.Context, c kubernetes.Interface, ns string) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		opts.LabelSelector = v1alpha1.LabelServiceName // only slices created by MCS controller
		return c.DiscoveryV1().EndpointSlices(ns).List(ctx, opts)
	}
}

func endpointSliceWatchFunc(ctx context.Context, c kubernetes.Interface, ns string) func(options meta.ListOptions) (watch.Interface, error) {
	return func(opts meta.ListOptions) (watch.Interface, error) {
		opts.LabelSelector = v1alpha1.LabelServiceName // only slices created by MCS controller
		return c.DiscoveryV1().EndpointSlices(ns).Watch(ctx, opts)
	}
}

func endpointSliceListFuncV1beta1(ctx context.Context, c kubernetes.Interface, ns string) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		opts.LabelSelector = v1alpha1.LabelServiceName // only slices created by MCS controller
		return c.DiscoveryV1beta1().EndpointSlices(ns).List(ctx, opts)
	}
}

func endpointSliceWatchFuncV1beta1(ctx context.Context, c kubernetes.Interface, ns string) func(options meta.ListOptions) (watch.Interface, error) {
	return func(opts meta.ListOptions) (watch.Interface, error) {
		opts.LabelSelector = v1alpha1.LabelServiceName // only slices created by MCS controller
		return c.DiscoveryV1beta1().EndpointSlices(ns).Watch(ctx, opts)
	}
}

// GetNamespaceByName returns the namespace by name. If nothing is found an error is returned.
func (c *control) GetNamespaceByName(name string) (*object.Namespace, error) {
	o, exists, err := c.nsLister.GetByKey(name)
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

func (c *control) Add(obj interface{})               { c.updateModified() }
func (c *control) Delete(obj interface{})            { c.updateModified() }
func (c *control) Update(oldObj, newObj interface{}) { c.detectChanges(oldObj, newObj) }

// detectChanges detects changes in objects, and updates the modified timestamp
func (c *control) detectChanges(oldObj, newObj interface{}) {
	// If both objects have the same resource version, they are identical.
	if newObj != nil && oldObj != nil && (oldObj.(meta.Object).GetResourceVersion() == newObj.(meta.Object).GetResourceVersion()) {
		return
	}
	obj := newObj
	if obj == nil {
		obj = oldObj
	}
	switch ob := obj.(type) {
	case *object.ServiceImport:
		c.updateModified()
	case *object.Endpoints:
		if !endpointsEquivalent(oldObj.(*object.Endpoints), newObj.(*object.Endpoints)) {
			c.updateModified()
		}
	default:
		log.Warningf("Updates for %T not supported.", ob)
	}
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

func (c *control) Modified() int64 {
	unix := atomic.LoadInt64(&c.modified)
	return unix
}

func (c *control) updateModified() {
	unix := time.Now().Unix()
	atomic.StoreInt64(&c.modified, unix)
}

func svcNameNamespaceIndexFunc(obj interface{}) ([]string, error) {
	s, ok := obj.(*object.ServiceImport)
	if !ok {
		return nil, errors.New("obj was not of the correct type")
	}
	return []string{s.Index}, nil
}

func epNameNamespaceIndexFunc(obj interface{}) ([]string, error) {
	s, ok := obj.(*object.Endpoints)
	if !ok {
		return nil, errors.New("obj was not of the correct type")
	}
	return []string{s.Index}, nil
}
